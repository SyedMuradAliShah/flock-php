import FlockKit
import SwiftUI

/// The output of a server Flock runs (a Node.js site's dev server, a proxy's command): the last
/// 300 lines of its service log, refreshed every second while `live`, with Clear and Open Log.
struct ServerOutput: View {
    @Environment(AppModel.self) private var model
    @Environment(\.openWindow) private var openWindow
    /// The service log's name.
    let log: String
    /// The server runs or starts, so its output grows.
    let live: Bool

    private let textState = State<String>(initialValue: "")

    /// What the refresh depends on: another log, or the server starting or stopping.
    private struct Refresh: Hashable {
        let log: String
        let live: Bool
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text("Output").foregroundStyle(.secondary)
                Spacer()
                Button("Clear") {
                    Task {
                        try? await model.client.clearServiceLog(log)
                        textState.wrappedValue = ""
                    }
                }
                Button("Open Log") {
                    model.logs.requested = .service(log)
                    openWindow.activating(WindowID.logs)
                }
            }
            .controlSize(.small)
            ScrollViewReader { proxy in
                ScrollView {
                    Text(textState.wrappedValue.isEmpty ? " " : textState.wrappedValue)
                        .font(.system(.caption, design: .monospaced))
                        .textSelection(.enabled)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(8)
                    Color.clear.frame(height: 1).id("end")
                }
                .background(Color(nsColor: .textBackgroundColor), in: RoundedRectangle(cornerRadius: 6))
                .overlay(RoundedRectangle(cornerRadius: 6).strokeBorder(.separator))
                .onChange(of: textState.wrappedValue, initial: true) { proxy.scrollTo("end", anchor: .bottom) }
            }
            .frame(height: 220)
        }
        .task(id: Refresh(log: log, live: live)) { await refresh() }
    }

    /// Reads the log once, and every second while the server is live (until the pane goes away).
    private func refresh() async {
        let client = model.client
        repeat {
            if let text = try? await client.serviceLog(log, lines: 300), text != textState.wrappedValue {
                textState.wrappedValue = text
            }
            if !live { return }
            try? await Task.sleep(for: .seconds(1))
        } while !Task.isCancelled
    }
}

/// A Node.js site's (or worktree's) dev server: its state, Start, Stop and Restart, the command it
/// runs, and its output.
struct NodeDevServerSection: View {
    @Environment(AppModel.self) private var model
    let site: Status.Site

    /// Start, Stop or Restart is on its way to the daemon.
    private let busyState = State<Bool>(initialValue: false)

    var body: some View {
        VStack(alignment: .leading, spacing: 20) {
            VStack(alignment: .leading, spacing: 10) {
                Text("Dev server").font(.subheadline.weight(.semibold))
                if let server = site.nodeServer {
                    let state = Self.state(server)
                    row("Status") {
                        VStack(alignment: .leading, spacing: 4) {
                            HStack(spacing: 6) {
                                StateDot(color: state.color)
                                Text(state.text).textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
                            }
                            if server.state != .failed, let problem = server.problem {
                                Text(problem).font(.caption).foregroundStyle(.secondary)
                                    .textSelection(.enabled)
                                    .fixedSize(horizontal: false, vertical: true)
                            }
                        }
                    }
                    HStack(spacing: 8) {
                        switch server.state {
                        case .sleeping, .failed:
                            Button("Start") { run(stop: false) }
                        case .starting:
                            Button("Stop") { run(stop: true) }
                        case .running:
                            Button("Stop") { run(stop: true) }
                            Button("Restart") { run(stop: false) }
                        }
                        if busyState.wrappedValue {
                            ProgressView().controlSize(.mini)
                        }
                    }
                    .disabled(busyState.wrappedValue)
                    .controlSize(.small)
                    row("Command") {
                        Text(server.command).font(.callout.monospaced()).foregroundStyle(.secondary)
                            .textSelection(.enabled)
                    }
                }
                Text("Flock starts the dev server when the site is first visited, and stops it after 30 minutes without requests. An open tab with hot reload keeps it running.")
                    .font(.caption).foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            if let server = site.nodeServer {
                ServerOutput(log: server.log, live: server.state == .starting || server.state == .running)
            }
        }
    }

    /// A labelled row, lined up like the panes' other settings.
    private func row<Content: View>(_ label: String, @ViewBuilder content: () -> Content) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 12) {
            Text(label).foregroundStyle(.secondary)
                .frame(width: 80, alignment: .leading)
            content()
            Spacer(minLength: 0)
        }
    }

    /// The dot and text of a dev server's state.
    static func state(_ server: Status.SiteNodeServer) -> (color: Color, text: String) {
        switch server.state {
        case .sleeping: (StateDot.stopped, "Sleeping · starts on the next visit")
        case .starting: (StateDot.busy, "Starting…")
        case .running: (StateDot.running, "Running on port \(server.port)")
        case .failed: (StateDot.failed, server.problem.map { "Stopped: \($0)" } ?? "Stopped")
        }
    }

    /// Starts (or restarts) or stops the dev server, by the site's (or worktree's) own name.
    private func run(stop: Bool) {
        let store = model.settings
        let name = site.name
        busyState.wrappedValue = true
        Task {
            await store.perform { client in
                if stop {
                    _ = try await client.stopSite(name)
                } else {
                    _ = try await client.restartSite(name)
                }
            }
            busyState.wrappedValue = false
        }
    }
}

/// A server's state as a dot: grey stopped or sleeping, amber on the way, green running, red failed.
struct StateDot: View {
    let color: Color

    var body: some View {
        Circle().fill(color).frame(width: 7, height: 7)
    }

    static let stopped = Color.secondary.opacity(0.5)
    static let busy = Color.orange
    static let running = Color.green
    static let failed = Color.red
}

/// Edits the command that starts a proxy's server, its folder and its stop command.
struct ProxyCommandSheet: View {
    let proxy: Status.Proxy
    let save: (ProxyChange) -> Void
    let cancel: () -> Void

    private let commandState: State<String>
    private let dirState: State<String>
    private let stopState: State<String>

    init(proxy: Status.Proxy, save: @escaping (ProxyChange) -> Void, cancel: @escaping () -> Void) {
        self.proxy = proxy
        self.save = save
        self.cancel = cancel
        commandState = State(initialValue: proxy.command?.command ?? "")
        // A new command runs in the home folder until another is chosen.
        dirState = State(initialValue: proxy.command?.dir ?? FileManager.default.homeDirectoryForCurrentUser.path)
        stopState = State(initialValue: proxy.command?.stopCommand ?? "")
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Command of \(proxy.name)").font(.headline)
            Form {
                TextField("Command", text: commandState.projectedValue, prompt: Text("npm run dev"))
                    .font(.callout.monospaced())
                LabeledContent("Folder") {
                    HStack(spacing: 8) {
                        Text(dirState.wrappedValue).font(.callout.monospaced())
                            .lineLimit(1).truncationMode(.middle)
                            .help(dirState.wrappedValue)
                        Button("Choose…") {
                            let start = URL(fileURLWithPath: dirState.wrappedValue)
                            if let url = SettingsStore.chooseDirectories(prompt: "Choose", multiple: false, in: start).first {
                                dirState.wrappedValue = url.path
                            }
                        }
                        .controlSize(.small)
                    }
                }
                TextField("Stop command", text: stopState.projectedValue, prompt: Text("docker compose stop"))
                    .font(.callout.monospaced())
            }
            Text("Flock runs the command when you click Start and shows its output here. For a command that starts the server in the background, like docker compose up -d, add the command that stops it.")
                .font(.caption).foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            HStack {
                Spacer()
                Button("Cancel", role: .cancel) { cancel() }
                    .keyboardShortcut(.cancelAction)
                Button("Save") {
                    let command = commandState.wrappedValue.trimmingCharacters(in: .whitespaces)
                    // An empty command removes it, with its folder and stop command.
                    save(command.isEmpty
                         ? ProxyChange(command: "")
                         : ProxyChange(command: command, dir: dirState.wrappedValue,
                                       stopCommand: stopState.wrappedValue.trimmingCharacters(in: .whitespaces)))
                }
                .keyboardShortcut(.defaultAction)
            }
        }
        .padding(20)
        .frame(width: 480)
    }
}
