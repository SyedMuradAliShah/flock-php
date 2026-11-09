package daemon

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"flock/internal/api"
	"flock/internal/config"
	"flock/internal/ports"
	"flock/internal/supervisor"
)

// A Node.js site's dev server (`npm run dev`) runs from the site's first request on: the web server
// tells the daemon of every request (see web.WakeFunc), which starts the server when it doesn't run.

// Node.js sites get a loopback port from this range: the one their name hashes to while it's
// free, so a site keeps its port across restarts without it being stored.
const (
	firstNodePort = 22000
	nodePorts     = 1000
)

// nodeCommand runs the project's dev script; npm passes what follows -- to it.
const nodeCommand = "npm run dev -- --port "

// nodeRuns are the Node.js sites' dev servers, by site name.
type nodeRuns struct {
	mu    sync.Mutex
	specs map[string]supervisor.Spec // every Node.js site's dev server, as of the last reload
	awake map[string]bool            // started by a request (or the apps) and kept running
	ports map[string]int             // the port each site got
}

// nodeProcess is the name of a Node.js site's dev server and of its log.
func nodeProcess(site string) string { return "node-" + site }

// preferredNodePort is the port a site's name hashes to.
func preferredNodePort(site string) int {
	h := fnv.New32a()
	h.Write([]byte(site))
	return firstNodePort + int(h.Sum32()%nodePorts)
}

// nodePort gives a site its port: the one it has while its server runs or the port is free, else
// the first free one from the port its name hashes to. taken are the ports of Flock's services and
// of the sites handled before it in this reload. reloadMu is held.
func (d *daemon) nodePort(site string, taken map[int]bool) int {
	r := &d.node
	r.mu.Lock()
	current := r.ports[site]
	r.mu.Unlock()
	if current != 0 && !taken[current] && (d.procUp(nodeProcess(site)) || ports.Free("127.0.0.1", current)) {
		return current
	}
	start := preferredNodePort(site)
	for i := range nodePorts {
		port := firstNodePort + (start-firstNodePort+i)%nodePorts
		if !taken[port] && ports.Free("127.0.0.1", port) {
			return port
		}
	}
	return start
}

// nodeSpec is a Node.js site's dev server. Its whole tree is stopped with it: npm's child holds
// the port. The shims on PATH pick the site's Node.js version for npm.
func (d *daemon) nodeSpec(cfg *config.Config, site config.Site, port int, aliases []string) supervisor.Spec {
	d.mu.Lock()
	restart := d.restarts[site.Name]
	d.mu.Unlock()
	env := d.commandEnv(append([]string{cfg.Domain(site)}, aliases...), "http://localhost:"+strconv.Itoa(port))
	if restart > 0 {
		env = append(env, "FLOCK_RESTART="+strconv.Itoa(restart)) // a new spec, so Ensure restarts it
	}
	return supervisor.Spec{
		Name: nodeProcess(site.Name), Shell: nodeCommand + strconv.Itoa(port), Dir: site.Path, Tree: true, Env: env,
		LogFile: filepath.Join(d.root, "log", nodeProcess(site.Name)+".log"),
	}
}

// hasDevScript reports whether the project's package.json has a dev script.
func hasDevScript(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return false
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	return json.Unmarshal(data, &pkg) == nil && pkg.Scripts["dev"] != ""
}

// keepNodeSites records the reload's Node.js sites, forgetting those that are gone.
func (r *nodeRuns) keep(specs map[string]supervisor.Spec, ports map[string]int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name := range r.awake {
		if _, ok := specs[name]; !ok {
			delete(r.awake, name)
		}
	}
	r.specs, r.ports = specs, ports
}

// isAwake reports whether a site's dev server is to run.
func (r *nodeRuns) isAwake(site string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.awake[site]
}

// wakeNode implements web.WakeFunc: it starts a Node.js site's dev server when it doesn't run.
// The web server calls it for every request, so it only starts the server in the background.
func (d *daemon) wakeNode(site string) (string, string) {
	if runningAsRoot() {
		return api.NodeFailed, "Flock runs as root here, and doesn't run a project's dev server with root's rights"
	}
	r := &d.node
	r.mu.Lock()
	_, known := r.specs[site]
	asleep := known && !r.awake[site]
	if asleep {
		if r.awake == nil {
			r.awake = map[string]bool{}
		}
		r.awake[site] = true
	}
	r.mu.Unlock()
	if !known {
		return "", ""
	}
	if asleep {
		go d.startNode(site)
		return api.NodeStarting, ""
	}
	state, problem := d.nodeState(site, false)
	return state, problem
}

// startNode starts an awake site's dev server, unless a reload did already.
func (d *daemon) startNode(site string) {
	d.reloadMu.Lock()
	defer d.reloadMu.Unlock()
	r := &d.node
	r.mu.Lock()
	spec, ok := r.specs[site]
	awake := r.awake[site]
	r.mu.Unlock()
	if ok && awake {
		d.procs.Ensure(spec)
	}
}

// nodeState is the state of a site's dev server; answering is whether its port answers (only
// asked for when that matters to the caller).
func (d *daemon) nodeState(site string, answering bool) (string, string) {
	if !d.node.isAwake(site) {
		return api.NodeSleeping, ""
	}
	for _, p := range d.procs.Status() {
		if p.Name != nodeProcess(site) {
			continue
		}
		switch {
		case !p.Up && p.Restarts > 0:
			return api.NodeFailed, nodeExitProblem(p.LastErr)
		case p.Up && answering:
			return api.NodeRunning, ""
		}
		return api.NodeStarting, ""
	}
	return api.NodeStarting, ""
}

// nodeExitProblem says why a dev server exited, from its process's last error.
func nodeExitProblem(lastErr string) string {
	lastErr = strings.TrimPrefix(lastErr, "<nil>")
	lastErr = strings.TrimPrefix(lastErr, ": ")
	if lastErr == "" {
		return "the dev server exited"
	}
	return lastErr
}

// liveNodeSites fills in the state of the Node.js sites' dev servers. The servers are copied:
// sites shares them with the last reload.
func (d *daemon) liveNodeSites(sites []api.SiteStatus) {
	for i, s := range sites {
		if s.NodeServer == nil {
			continue
		}
		n := *s.NodeServer
		n.State, n.Problem = d.nodeState(s.Name, answers("http://localhost:"+strconv.Itoa(n.Port)))
		if n.State == api.NodeFailed {
			if _, err := os.Stat(filepath.Join(s.Path, "node_modules")); err != nil {
				n.Problem = "node_modules is missing: run npm install in the project"
			}
		}
		if n.Problem == "" {
			n.Problem = s.NodeServer.Problem
		}
		sites[i].NodeServer = &n
	}
}

// nodeSiteProblem is what keeps a Node.js site from starting, found on reload.
func nodeSiteProblem(site config.Site) string {
	if runningAsRoot() {
		return "Flock runs as root here, and doesn't run a project's dev server with root's rights"
	}
	if !hasDevScript(site.Path) {
		return fmt.Sprintf("%s has no dev script in package.json for Flock to run", site.Name)
	}
	return ""
}
