using Flock.Kit;
using Flock.Tray.Infrastructure;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
namespace Flock.Tray.Settings;

/// <summary>
///     phpMyAdmin, under "Database Client", laid out like the catalog services: Folder… and Address… next to
///     each other like Port…, a Versions list with Update and ⋯, the address and folder with their open buttons,
///     and the "Last checked" footer. The daemon installs it from phpmyadmin.net and serves it as phpmyadmin.test.
/// </summary>
internal sealed partial class ServicesPane
{
    private PhpMyAdminState? phpMyAdmin;
    private string? phpMyAdminError;
    private string? phpMyAdminChosenFolder;

    private bool PhpMyAdminWorking => phpMyAdmin?.IsBusy == true;

    private async Task RefreshPhpMyAdminAsync(CancellationToken token)
    {
        try
        {
            var next = await Model.Client.PhpMyAdminAsync(token);
            if (!SamePhpMyAdmin(phpMyAdmin, next))
            {
                phpMyAdmin = next;
                if (rows.SelectedId == "phpmyadmin")
                {
                    rows.NotifySelectedServiceChanged();
                }
            }
        }
        catch (OperationCanceledException)
        {
        }
        catch (Exception error) when (!token.IsCancellationRequested)
        {
            App.Log(error);
        }
    }

    private static bool SamePhpMyAdmin(PhpMyAdminState? left, PhpMyAdminState right)
    {
        return left is not null && left.Installed == right.Installed && left.Version == right.Version && left.Folder == right.Folder && left.Name == right.Name &&
               left.Url == right.Url && left.Latest == right.Latest && left.Update == right.Update && left.CheckedAt == right.CheckedAt &&
               left.CheckError == right.CheckError && left.Job?.State == right.Job?.State && left.Job?.Phase == right.Job?.Phase &&
               left.Job?.Progress == right.Job?.Progress && left.Job?.Error == right.Job?.Error && left.Job?.Version == right.Job?.Version;
    }

    private async Task RunPhpMyAdminAsync(Func<CancellationToken, Task<PhpMyAdminState>> action)
    {
        phpMyAdminError = null;
        try
        {
            phpMyAdmin = await action(CancellationToken.None);
        }
        catch (Exception error)
        {
            App.Log(error);
            phpMyAdminError = error.Message;
        }
        Kick();
        rows.NotifySelectedServiceChanged();
    }

    private void PhpMyAdminDetail(StackPanel panel, Service service)
    {
        if (phpMyAdmin is not { } state)
        {
            panel.Children.Add(LoadingRow("Loading…"));
            return;
        }
        if (phpMyAdminError is { Length: > 0 } error)
        {
            var text = Ui.Caption(error, Ui.Critical);
            text.IsTextSelectionEnabled = true;
            panel.Children.Add(Ui.Row(6, Ui.Icon(Ui.GlyphWarning, 12, Ui.Critical), text));
        }
        if (state.Installed)
        {
            var actions = Ui.Row(8,
                Ui.Button("Folder…", () => _ = MovePhpMyAdminAsync(state), tooltip: "Move phpMyAdmin to another folder; it keeps working."),
                Ui.Button("Address…", () => _ = ChangePhpMyAdminAddressAsync(state), tooltip: "Serve phpMyAdmin at another name, like db" + state.Suffix));
            Ui.SetEnabled(actions, !state.IsBusy);
            panel.Children.Add(actions);
        }
        if (state.Job is { } job)
        {
            panel.Children.Add(PhpMyAdminJobCard(state, job));
        }
        else if (!state.Installed)
        {
            panel.Children.Add(PhpMyAdminInstallCard(state, service));
        }
        else
        {
            panel.Children.Add(PhpMyAdminVersions(state));
        }
        if (state.Installed)
        {
            panel.Children.Add(Ui.Column(6,
                Ui.Spread(Ui.Text("Address", "BodyStrongTextBlockStyle"),
                    Ui.IconButton(Ui.GlyphOpen, $"Open {state.Url}", () => Model.Open(state.Url))),
                Ui.CodeBlock(state.Url)));
            panel.Children.Add(Ui.Column(6,
                Ui.Spread(Ui.Text("Folder", "BodyStrongTextBlockStyle"), Ui.Button("Open", () => Model.Reveal(state.Folder))),
                Ui.CodeBlock(state.Folder)));
        }
        panel.Children.Add(PhpMyAdminFooter(state));
        panel.Children.Add(Ui.Caption("It connects to the MySQL or MariaDB you installed in Flock. Log in as root with an empty password. "
                                      + $"While it is installed, its name ({state.Name}) belongs to it: a folder with that name is served as {state.Name}-1."));
    }

    /// <summary>Nothing installed yet: the version (the newest, as there is no choice) and the folder, then Install in the footer.</summary>
    private Border PhpMyAdminInstallCard(PhpMyAdminState state, Service service)
    {
        var form = Ui.Column(10);
        var picker = new ComboBox { Header = "Version", MinWidth = 220, IsEnabled = false };
        picker.Items.Add(new ComboBoxItem { Content = state.Latest.Length > 0 ? $"{PhpMyAdminLine(state.Latest)} ({state.Latest})" : "Newest release" });
        picker.SelectedIndex = 0;
        form.Children.Add(picker);

        var folder = phpMyAdminChosenFolder ?? state.DefaultFolder;
        form.Children.Add(Ui.Column(6,
            Ui.Text("Install into", "BodyStrongTextBlockStyle", wrap: false),
            Ui.CodeBlock(folder),
            Ui.Row(8,
                Ui.Button("Choose folder…", () => _ = ChooseInstallFolderAsync()),
                Ui.Button("Use default", () =>
                {
                    phpMyAdminChosenFolder = null;
                    rows.NotifySelectedServiceChanged();
                }, tooltip: "Flock's own data folder"))));
        UIElement[] actions = state.Estimate.Length > 0 ? [form, Ui.Caption(Capitalize(state.Estimate), wrap: false)] : [form];
        return Hero(Ui.GlyphServer, "Install phpMyAdmin", service.Description, actions);
    }

    /// <summary>The installed version in one row, like a catalog service's lines: Update when there is one, ⋯ for the rest.</summary>
    private StackPanel PhpMyAdminVersions(PhpMyAdminState state)
    {
        var title = Ui.Row(8, Ui.Text($"phpMyAdmin {PhpMyAdminLine(state.Version)}", "BodyStrongTextBlockStyle", wrap: false), Ui.Badge("Active"));
        var facts = state.Version + (state.Update ? $" · {state.Latest} available" : state.Latest.Length > 0 ? " · up to date" : "");
        var info = Ui.Column(2, title, Ui.Caption(facts, state.Update ? Ui.Accent : null, false));

        var actions = Ui.Row(8);
        if (state.Update)
        {
            actions.Children.Add(Ui.Button($"Update to {state.Latest}", () => _ = RunPhpMyAdminAsync(token => Model.Client.InstallPhpMyAdminAsync(null, false, token)), true,
                Ui.GlyphDownload, $"Installs {state.Latest} in place of {state.Version}. Your settings stay."));
        }
        var more = new MenuFlyout();
        more.Items.Add(Item($"Reinstall {state.Version}", () => _ = ReinstallPhpMyAdminAsync(state)));
        more.Items.Add(Item("Uninstall…", () => _ = UninstallPhpMyAdminAsync(state)));
        var moreButton = Ui.IconButton(Ui.GlyphMore, "More", () => { });
        moreButton.Flyout = more;
        actions.Children.Add(moreButton);
        Ui.SetEnabled(actions, !state.IsBusy);

        return Ui.Column(6, Ui.Text("Versions", "BodyStrongTextBlockStyle"), VersionRows([VersionRow(info, actions)]),
            Ui.Caption("Updates keep your settings."));
    }

    /// <summary>"Last checked …" with Check for Updates, or Install / Update when there is something to install.</summary>
    private StackPanel PhpMyAdminFooter(PhpMyAdminState state)
    {
        var text = state.CheckedAt is { } at ? $"Last checked {Ui.Relative(at)}" : "Not checked yet";
        UIElement trailing;
        if (state.IsBusy)
        {
            trailing = Ui.Row(6, new ProgressRing { Width = 14, Height = 14 }, Ui.Caption(state.Job!.PhaseText, wrap: false));
        }
        else if (!state.Installed)
        {
            trailing = Ui.Button(state.Latest.Length > 0 ? $"Install phpMyAdmin {state.Latest}" : "Install phpMyAdmin",
                () => _ = RunPhpMyAdminAsync(token => Model.Client.InstallPhpMyAdminAsync(phpMyAdminChosenFolder, false, token)), true, Ui.GlyphDownload,
                "Downloads the newest phpMyAdmin from phpmyadmin.net and checks it.");
        }
        else if (state.Update)
        {
            trailing = Ui.Button($"Update to {state.Latest}", () => _ = RunPhpMyAdminAsync(token => Model.Client.InstallPhpMyAdminAsync(null, false, token)), true, Ui.GlyphDownload);
        }
        else
        {
            trailing = Ui.Link("Check for Updates", () => _ = RunPhpMyAdminAsync(token => Model.Client.CheckPhpMyAdminAsync(token)));
        }
        var footer = Ui.Column(2, Ui.Spread(Ui.Caption(text, wrap: false), trailing));
        if (state.CheckError.Length > 0)
        {
            var error = Ui.Caption($"Couldn't check: {state.CheckError}", Ui.Caution);
            error.MaxLines = 2;
            ToolTipService.SetToolTip(error, state.CheckError);
            footer.Children.Add(Ui.Row(6, Ui.Icon(Ui.GlyphWarning, 12, Ui.Caution), error));
        }
        return footer;
    }

    /// <summary>"5.2" for 5.2.3, like the lines of the catalog services.</summary>
    private static string PhpMyAdminLine(string version)
    {
        var parts = version.Split('.');
        return parts.Length >= 2 ? $"{parts[0]}.{parts[1]}" : version;
    }

    private async Task ChooseInstallFolderAsync()
    {
        if (await PickPhpMyAdminFolderAsync() is { } folder)
        {
            phpMyAdminChosenFolder = folder;
            rows.NotifySelectedServiceChanged();
        }
    }

    private async Task<string?> PickPhpMyAdminFolderAsync()
    {
        var picked = await Window.PickFolder();
        if (string.IsNullOrEmpty(picked))
        {
            return null;
        }
        return string.Equals(Path.GetFileName(picked.TrimEnd('\\', '/')), "phpmyadmin", StringComparison.OrdinalIgnoreCase)
            ? picked
            : Path.Combine(picked, "phpmyadmin");
    }

    private Border PhpMyAdminJobCard(PhpMyAdminState state, PhpMyAdminJob job)
    {
        var content = Ui.Column(8);
        var target = job.Version.Length > 0 ? $"phpMyAdmin {job.Version}" : "phpMyAdmin";
        if (job.IsRunning)
        {
            var percent = job.HasProgress ? $"{Math.Round(job.Progress * 100)} %" : "";
            content.Children.Add(Ui.Spread(Ui.Text($"{(state.Installed ? "Updating" : "Installing")} {target}", "BodyStrongTextBlockStyle"), Ui.Caption(percent, wrap: false)));
            var phase = job.PhaseText + (job.StartedAt is { Year: > 2000 } started ? $" · started {Ui.Relative(started)}" : "");
            content.Children.Add(new Controls.OperationProgress { Text = phase, Value = job.Progress, IsIndeterminate = !job.HasProgress });
            content.Children.Add(Ui.Button("Cancel", () => _ = RunPhpMyAdminAsync(token => Model.Client.CancelPhpMyAdminJobAsync(token))));
            return Inner(content);
        }
        content.Children.Add(Ui.Row(6, Ui.Icon(Ui.GlyphWarning, 14, Ui.Critical), Ui.Text($"Installing {target} failed", "BodyStrongTextBlockStyle")));
        var error = Ui.Caption(job.Error.Length > 0 ? job.Error : "Flock reported no details.", Ui.Critical);
        error.IsTextSelectionEnabled = true;
        error.MaxLines = 4;
        ToolTipService.SetToolTip(error, job.Error);
        content.Children.Add(error);
        content.Children.Add(Ui.Row(8,
            Ui.Button("Try Again", () => _ = RunPhpMyAdminAsync(async token =>
            {
                await Model.Client.DismissPhpMyAdminJobAsync(token);
                return await Model.Client.InstallPhpMyAdminAsync(phpMyAdminChosenFolder, false, token);
            }), true, Ui.GlyphRefresh),
            Ui.Button("Dismiss", () => _ = RunPhpMyAdminAsync(token => Model.Client.DismissPhpMyAdminJobAsync(token)))));
        return Inner(content);
    }

    private async Task MovePhpMyAdminAsync(PhpMyAdminState state)
    {
        var target = await PickPhpMyAdminFolderAsync();
        if (string.IsNullOrEmpty(target) || string.Equals(target, state.Folder, StringComparison.OrdinalIgnoreCase))
        {
            return;
        }
        if (!await Window.Confirm("Move phpMyAdmin?", $"phpMyAdmin moves from {state.Folder} to {target}. It is unavailable for a moment while its files move.", "Move"))
        {
            return;
        }
        await RunPhpMyAdminAsync(token => Model.Client.SetPhpMyAdminFolderAsync(target, token));
    }

    private async Task ChangePhpMyAdminAddressAsync(PhpMyAdminState state)
    {
        var typed = await Window.Ask("Change phpMyAdmin's address",
            $"Type the name to serve it at, like db for db{state.Suffix}. A name that a site of yours already uses is refused; your sites are never renamed.",
            $"db{state.Suffix}", "Change", state.Name);
        if (string.IsNullOrWhiteSpace(typed) || string.Equals(typed.Trim(), state.Name, StringComparison.OrdinalIgnoreCase)
            || string.Equals(typed.Trim(), state.Domain, StringComparison.OrdinalIgnoreCase))
        {
            return;
        }
        await RunPhpMyAdminAsync(token => Model.Client.SetPhpMyAdminNameAsync(typed.Trim(), token));
    }

    private async Task ReinstallPhpMyAdminAsync(PhpMyAdminState state)
    {
        if (await Window.Confirm($"Reinstall phpMyAdmin {state.Version}?",
                $"Flock downloads it again and replaces its files in {state.Folder}, which is being served right now. Your config.user.inc.php stays.", "Reinstall", true))
        {
            await RunPhpMyAdminAsync(token => Model.Client.InstallPhpMyAdminAsync(null, true, token));
        }
    }

    private async Task UninstallPhpMyAdminAsync(PhpMyAdminState state)
    {
        if (await Window.Confirm("Uninstall phpMyAdmin?",
                $"Flock removes phpMyAdmin from {state.Folder} and stops serving {state.Domain}. Your databases aren't touched.", "Uninstall", true))
        {
            phpMyAdminChosenFolder = null;
            await RunPhpMyAdminAsync(token => Model.Client.UninstallPhpMyAdminAsync(token));
        }
    }
}
