# phpMyAdmin

Flock installs [phpMyAdmin](https://www.phpmyadmin.net) for you and serves it like a site, at `http://phpmyadmin.test`. It connects to the MySQL and MariaDB you installed in Flock, so there's no web server, PHP version or database setting to configure.

phpMyAdmin is downloaded from phpmyadmin.net, and its SHA-256 checksum is verified before it's installed.

## Installing

:::app
Open **Settings…** from the Flock panel and go to **Services**. phpMyAdmin is under **Database Client**; switch the filter from **Installed** to **All** to see it. Choose where to install it, then click **Install phpMyAdmin**.

On Windows, once it's installed, **Address** has an arrow button that opens it in your browser.

The Services screen for phpMyAdmin is available on Windows. On macOS and Linux, use the CLI below.
:::

:::cli
```bash
flock phpmyadmin                       # show whether it's installed, where, and at which address
flock phpmyadmin install               # install it (the newest release; also updates it)
flock phpmyadmin install --folder ~/tools/phpmyadmin
```
:::

By default it goes into a `phpmyadmin` folder in [Flock's data directory](configuration.md), the same place Flock keeps its other downloads. Choose another folder if you'd rather keep it elsewhere; the folder has to be empty or new, because Flock deletes only what it put there when you uninstall.

phpMyAdmin 5.2 supports PHP versions up to 8.3. Flock runs it on your default PHP when that works, and otherwise on the newest installed version that does. If you have none, Flock tells you which versions to install.

## Signing in

While the `root` user has no password, which is how Flock sets up MySQL and MariaDB, phpMyAdmin signs you in by itself. Once `root` has a password, phpMyAdmin shows its login form and asks for it.

Flock decides this each time a page loads, so it also follows a password you changed with another tool. If you have several MySQL or MariaDB services, each is listed in phpMyAdmin's server list.

To add your own phpMyAdmin settings, create `config.user.inc.php` in the version's folder. Flock rewrites `config.inc.php` whenever its configuration changes, but never touches your file, and keeps it when phpMyAdmin updates.

## The address

phpMyAdmin is a site named `phpmyadmin`, so it's at `phpmyadmin.test` (with your [TLD](domains.md)). While it's installed, that name belongs to it: a folder called `phpmyadmin` is served as `phpmyadmin-1` instead, and the same goes for linking a folder under that name. It doesn't appear in the site list, because it has a screen of its own.

You can serve it under another name:

:::app
Click **Address…** and type a name, like `db`. **Back to phpmyadmin.test** restores the default.
:::

:::cli
```bash
flock phpmyadmin name db       # http://db.test
flock phpmyadmin name          # back to phpmyadmin.test
```
:::

A name that a site, proxy or alias already uses is refused. Flock never renames your sites to make room.

## Moving it

:::app
Click **Folder…** to move phpMyAdmin to another folder. It keeps working, and keeps its address and settings.
:::

:::cli
```bash
flock phpmyadmin folder ~/tools/phpmyadmin
flock phpmyadmin folder                # back to Flock's data directory
```
:::

## Updating

Flock checks phpmyadmin.net for a newer release every few hours. When there is one, the update is offered; when you're on the newest, there's nothing to update.

:::app
Click **Update to …** in the **Versions** list, or **Check for Updates** to look now. **Reinstall** in the **⋯** menu downloads the same version again and replaces its files in place, which is useful if one was damaged.
:::

:::cli
```bash
flock phpmyadmin install               # update to the newest release
flock phpmyadmin install --reinstall   # replace the files of the installed version
```
:::

## Uninstalling

:::app
Choose **Uninstall…** in the **⋯** menu. Your databases aren't touched.
:::

:::cli
```bash
flock phpmyadmin uninstall
```
:::
