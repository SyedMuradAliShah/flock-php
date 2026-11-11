using Flock.Kit;
using Flock.Tray.Features.Services;
using Flock.Tray.Infrastructure;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using CatalogService = Flock.Kit.CatalogService;
namespace Flock.Tray.Settings;

/// <summary>
///     The catalog services (MySQL, MariaDB, Redis, MongoDB, Meilisearch, RustFS): pick a version line
///     and a port and install, follow the install (a job in the daemon); like PostgreSQL's majors,
///     lines install side by side: install more, switch, update or uninstall them. Each keeps its own data.
/// </summary>
internal sealed partial class ServicesPane
{
    private bool CatalogWorking(string id)
    {
        return Model.Catalog.IsRunning(id) || Model.Catalog.Current?[id]?.Job?.IsRunning == true;
    }

    private bool CatalogFailed(Service service)
    {
        return Model.Catalog.Current?[service.Id]?.Job?.IsFailed == true;
    }

    private void CatalogDetail(StackPanel panel, Service service)
    {
        if (Model.Catalog.Current is not { } catalog)
        {
            panel.Children.Add(LoadingRow("Loading…"));
            return;
        }
        if (catalog[service.Id] is not { } entry)
        {
            panel.Children.Add(Ui.Caption($"This Flock daemon doesn't offer {service.Name}. Update Flock, then try again."));
            return;
        }
        var row = Model.ReleaseActions.ResolveRow(entry.Id);
        if (!row.IsAvailable)
        {
            panel.Children.Add(Ui.Caption(entry.Unavailable.Length > 0 ? entry.Unavailable : $"{entry.Name} isn't available on this computer."));
            panel.Children.Add(CheckFooter(service.Id, Model.ReleaseActions.Resolve(service.Id)));
            return;
        }
        if (row.Job is { } job)
        {
            panel.Children.Add(CatalogJobCard(entry, row, job));
        }
        if (!entry.Lines.Any(l => l.Installed))
        {
            if (entry.Job is null && !Model.Catalog.IsRunning(entry.Id))
            {
                var targets = Model.ReleaseActions.InstallTargets(entry.Id);
                panel.Children.Add(targets.Count == 0
                    ? Inner(Ui.Caption($"Flock offers no {entry.Name} versions for this computer."))
                    : CatalogInstallCard(entry, row, targets));
            }
        }
        else
        {
            panel.Children.Add(CatalogLines(entry, row));
        }
        if (entry.AcceptsExisting)
        {
            panel.Children.Add(ExistingVersionRow(entry));
        }
        if (entry.HasRootUser && entry.IsInstalled)
        {
            panel.Children.Add(RootPasswordRow(entry, service));
        }
        panel.Children.Add(CheckFooter(service.Id, Model.ReleaseActions.Resolve(service.Id)));
    }

    /// <summary>Use a version the user already has (installed by another tool or by hand), from its own base and data directories.</summary>
    private StackPanel ExistingVersionRow(CatalogService entry)
    {
        var add = Ui.Button("Add…", () => _ = AddExistingVersionAsync(entry),
            tooltip: $"Run a {entry.Name} that is already on your computer, from its own folders.");
        add.IsEnabled = !CatalogWorking(entry.Id);
        return Ui.Column(6,
            Ui.Spread(Ui.Text("Existing version", "BodyStrongTextBlockStyle"), add),
            Ui.Caption($"Already have {entry.Name} on this computer? Choose its base directory and its data directory, and Flock runs it in place, without downloading anything."));
    }

    private async Task AddExistingVersionAsync(CatalogService entry)
    {
        var binary = new TextBox { Header = "Base directory (basedir)", PlaceholderText = $@"C:\path\to\{entry.Id}" };
        var data = new TextBox { Header = "Data directory (datadir)", PlaceholderText = $@"C:\path\to\{entry.Id}\data" };
        var dialog = new ContentDialog
        {
            Title = $"Add an existing {entry.Name}",
            Content = Ui.Column(12,
                Ui.Caption("The base directory is where the server is installed: it has a bin folder inside it. The data directory is where its databases are stored. Flock runs the server on that data in place, and never deletes either one. You can open both later from the version's ⋯ menu."),
                FolderPickerRow(binary), FolderPickerRow(data),
                Ui.Caption("Stop whatever else is using that data first. A newer server upgrades the data files on its first start, and an older program may not be able to read them afterwards, so back the data directory up if you need to go back.", Ui.Caution)),
            PrimaryButtonText = "Add", CloseButtonText = "Cancel", DefaultButton = ContentDialogButton.Primary, IsPrimaryButtonEnabled = false, XamlRoot = Window.XamlRoot
        };
        void Changed(object sender, TextChangedEventArgs args) => dialog.IsPrimaryButtonEnabled = binary.Text.Trim().Length > 0 && data.Text.Trim().Length > 0;
        binary.TextChanged += Changed;
        data.TextChanged += Changed;
        if (await dialog.ShowAsync() != ContentDialogResult.Primary)
        {
            return;
        }
        await RunCatalogAsync(entry.Id, () => Model.Catalog.AddExistingVersionAsync(entry.Id, binary.Text.Trim(), data.Text.Trim()));
    }

    /// <summary>A path box with a Browse button next to it.</summary>
    private Grid FolderPickerRow(TextBox box)
    {
        var browse = Ui.Button("Browse…", async () =>
        {
            if (await Window.PickFolder() is { Length: > 0 } picked)
            {
                box.Text = picked;
            }
        });
        browse.VerticalAlignment = VerticalAlignment.Bottom;
        var row = new Grid { ColumnSpacing = 8 };
        row.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
        row.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
        Grid.SetColumn(browse, 1);
        row.Children.Add(box);
        row.Children.Add(browse);
        return row;
    }

    /// <summary>The root user's password: none, as Flock sets it up, or the one the user chose.</summary>
    private StackPanel RootPasswordRow(CatalogService entry, Service service)
    {
        var note = Ui.Caption(entry.RootPassword
            ? "Set. phpMyAdmin asks for it when you sign in."
            : "None. The root user signs in without one, so phpMyAdmin signs in by itself.");
        var copy = Ui.IconButton(Ui.GlyphCopy, "Copy the root password", () => _ = CopyRootPasswordAsync(entry, null, note));
        copy.IsEnabled = entry.RootPassword;
        if (!entry.RootPassword)
        {
            ToolTipService.SetToolTip(copy, "There is no password to copy.");
        }
        var change = Ui.Button("Change…", () => _ = ChangeRootPasswordAsync(entry),
            tooltip: service.Running ? "Set any password, or leave it empty for none." : $"Start {entry.Name} first.");
        change.IsEnabled = service.Running && !CatalogWorking(entry.Id);
        var row = Ui.Column(6, Ui.Spread(Ui.Text($"Root password · {entry.Name} {entry.Line}", "BodyStrongTextBlockStyle", wrap: false), Ui.Row(8, copy, change)), note);
        if (entry.Lines.Count(l => l.Installed) > 1)
        {
            row.Children.Add(Ui.Caption("Every installed version has its own root password. Change another one after switching to it; copy any version's from its ⋯ menu."));
        }
        return row;
    }

    /// <summary>Copies the root password Flock has for a version (null: the running one).</summary>
    private async Task CopyRootPasswordAsync(CatalogService entry, string? line, TextBlock? note)
    {
        try
        {
            var password = await Model.Catalog.ReadRootPasswordAsync(entry.Id, line);
            if (password.Length == 0)
            {
                await Ui.Inform(Window.XamlRoot, "No root password", $"{entry.Name} {line ?? entry.Line} has no root password to copy.");
                return;
            }
            Ui.Copy(password);
            if (note is not null)
            {
                note.Text = "Copied to the clipboard.";
            }
        }
        catch (Exception error)
        {
            App.Log(error);
            await Ui.Inform(Window.XamlRoot, "Couldn't copy the password", error.Message);
        }
    }

    private async Task ChangeRootPasswordAsync(CatalogService entry)
    {
        var password = new PasswordBox { Header = "New password", PlaceholderText = "Leave empty for no password" };
        var repeat = new PasswordBox { Header = "Repeat the new password" };
        var current = new PasswordBox { Header = "Current password", PlaceholderText = "Only if it isn't the one Flock set" };
        var mismatch = Ui.Caption("The two passwords differ.", Ui.Critical);
        mismatch.Visibility = Visibility.Collapsed;
        var dialog = new ContentDialog
        {
            Title = $"{entry.Name} root password",
            Content = Ui.Column(12,
                Ui.Caption("Applies to the root user of this server. Update the password in your projects' .env files too; with none, phpMyAdmin signs in by itself."),
                password, repeat, mismatch, current),
            PrimaryButtonText = "Change", CloseButtonText = "Cancel", DefaultButton = ContentDialogButton.Primary, XamlRoot = Window.XamlRoot
        };
        dialog.PrimaryButtonClick += (_, args) =>
        {
            if (password.Password != repeat.Password)
            {
                mismatch.Visibility = Visibility.Visible;
                args.Cancel = true;
            }
        };
        if (await dialog.ShowAsync() != ContentDialogResult.Primary)
        {
            return;
        }
        await RunCatalogAsync(entry.Id, () => Model.Catalog.SetRootPasswordAsync(entry.Id, current.Password, password.Password));
    }

    /// <summary>What it is, the version line and port, and Install (see InstallCard).</summary>
    private Border CatalogInstallCard(CatalogService entry, ServiceReleaseRow row, IReadOnlyList<ServiceReleaseTarget> targets)
    {
        return InstallCard(entry.Id, entry.Name, entry.Description.Length > 0 ? entry.Description : "Runs on 127.0.0.1 for your sites.",
            targets, row.Port, row.Port != row.DefaultPort ? $"Port {row.DefaultPort} is taken, so Flock suggests {row.Port}." : null, row.Estimate);
    }

    /// <summary>A running or failed install.</summary>
    private Border CatalogJobCard(CatalogService entry, ServiceReleaseRow row, CatalogJob job)
    {
        var content = Ui.Column(8);
        var actions = Ui.Row(8);
        var target = job.Version.Length > 0 ? $"{entry.Name} {job.Version}" : entry.Name;
        if (job.IsRunning)
        {
            var verb = job.Action == "update" ? "Updating" : "Installing";
            var percent = job.HasProgress ? $"{Math.Round(job.Progress * 100)} %" : "";
            content.Children.Add(Ui.Spread(Ui.Text($"{verb} {target}", "BodyStrongTextBlockStyle"), Ui.Caption(percent, wrap: false)));
            var phase = job.PhaseText + (job.StartedAt is { Year: > 2000 } started ? $" · started {Ui.Relative(started)}" : "");
            content.Children.Add(new Controls.OperationProgress { Text = phase, Value = job.Progress, IsIndeterminate = !job.HasProgress });
            foreach (var command in row.JobCommands)
            {
                actions.Children.Add(CatalogJobCommand(entry, job, command));
            }
            content.Children.Add(actions);
            return Inner(content);
        }

        content.Children.Add(Ui.Row(6, Ui.Icon(Ui.GlyphWarning, 14, Ui.Critical),
            Ui.Text(job.Action == "update" ? $"Updating {entry.Name} failed" : $"Installing {target} failed", "BodyStrongTextBlockStyle")));
        var error = Ui.Caption(job.Error.Length > 0 ? job.Error : "Flock reported no details; the log may tell more.", Ui.Critical);
        error.IsTextSelectionEnabled = true;
        error.MaxLines = 4;
        ToolTipService.SetToolTip(error, job.Error);
        content.Children.Add(error);
        foreach (var command in row.JobCommands)
        {
            actions.Children.Add(CatalogJobCommand(entry, job, command));
        }
        content.Children.Add(actions);
        return Inner(content);
    }

    /// <summary>Every line, like PostgreSQL's majors: install next to the others, switch, update, uninstall.</summary>
    private StackPanel CatalogLines(CatalogService entry, ServiceReleaseRow row)
    {
        var rows = new List<UIElement>();
        foreach (var line in row.Lines)
        {
            var source = entry.Lines.First(candidate => candidate.Id == line.Target.Line);
            var title = Ui.Row(8, Ui.Text($"{entry.Name} {line.Target.Label}", line.IsInstalled ? "BodyStrongTextBlockStyle" : "BodyTextBlockStyle", wrap: false));
            if (line.IsActive)
            {
                title.Children.Add(Ui.Badge("Active"));
            }
            var info = Ui.Column(2, title, Ui.Caption(line.Facts, line.HasUpdate ? Ui.Accent : null, false));
            if (source.Yanked.Length > 0)
            {
                var withdrawn = Ui.Caption(line.HasUpdate ? $"Withdrawn: {source.Yanked} — update to replace it" : $"Withdrawn: {source.Yanked}", Ui.Caution);
                ToolTipService.SetToolTip(withdrawn, "This build was withdrawn. Flock replaces it as soon as a fixed build is published.");
                info.Children.Add(Ui.Row(6, Ui.Icon(Ui.GlyphWarning, 12, Ui.Caution), withdrawn));
            }

            var actions = Ui.Row(8);
            foreach (var command in line.PrimaryCommands)
            {
                actions.Children.Add(CatalogCommandButton(entry, source, line, command));
            }
            if (line.MoreCommands.Count > 0 || source.RootPassword || source.BaseDir.Length > 0 || source.DataDir.Length > 0)
            {
                var more = new MenuFlyout();
                if (source.BaseDir.Length > 0)
                {
                    more.Items.Add(Item("Open Base Directory", () => Model.Reveal(source.BaseDir)));
                }
                if (source.DataDir.Length > 0)
                {
                    more.Items.Add(Item("Open Data Directory", () => Model.Reveal(source.DataDir)));
                }
                if (source.RootPassword)
                {
                    more.Items.Add(Item("Copy Root Password", () => _ = CopyRootPasswordAsync(entry, source.Id, null)));
                }
                foreach (var command in line.MoreCommands)
                {
                    switch (command)
                    {
                        case ServiceRowCommandKind.Activate:
                            more.Items.Add(Item("Switch…", () => _ = SwitchCatalog(entry, source, line.Target)));
                            break;
                        case ServiceRowCommandKind.Uninstall:
                            more.Items.Add(Item("Uninstall…", () => _ = UninstallCatalog(entry, source, line.Target)));
                            break;
                        case ServiceRowCommandKind.Reinstall:
                            more.Items.Add(Item($"Reinstall {source.Version}", () => _ = RunCatalogAsync(entry.Id,
                                () => Model.ReleaseActions.RunAsync(line.Target, command))));
                            break;
                        case ServiceRowCommandKind.ShowLog:
                            more.Items.Add(Item("Show Last Log", () => _ = ShowCatalogLog(entry)));
                            break;
                    }
                }
                var moreButton = Ui.IconButton(Ui.GlyphMore, "More", () => { });
                moreButton.Flyout = more;
                actions.Children.Add(moreButton);
            }
            rows.Add(VersionRow(info, actions));
        }
        return Ui.Column(6, Ui.Text("Versions", "BodyStrongTextBlockStyle"), VersionRows(rows),
            Ui.Caption(entry.Lines.Count > 1
                ? "Installed versions stay side by side. One runs at a time; each keeps its own data, and updates within a version keep it."
                : "Updates keep your data."));
    }

    private Button CatalogCommandButton(CatalogService entry, CatalogLine source, ServiceReleaseLine line, ServiceRowCommandKind command)
    {
        return command switch
        {
            ServiceRowCommandKind.Install => Ui.Button("Install", () => _ = RunCatalogAsync(entry.Id, () => Model.ReleaseActions.RunAsync(line.Target, command)),
                glyph: Ui.GlyphDownload, tooltip: (source.Latest.Length > 0 ? $"Downloads {entry.Name} {source.Latest}." : "Downloads the newest release.")
                                                  + (entry.IsInstalled ? $" {entry.Name} {entry.Line} keeps running until you switch." : "")
                                                  + (source.HasData ? " Its data is still here and comes back." : "")),
            ServiceRowCommandKind.Update => Ui.Button($"Update to {source.Latest}", () => _ = RunCatalogAsync(entry.Id,
                    () => Model.ReleaseActions.RunAsync(line.Target, command)), true, Ui.GlyphDownload,
                $"Installs {source.Latest} in place of {source.Version}. Your data stays{(source.Active ? $"; {entry.Name} restarts for a moment" : "")}."),
            ServiceRowCommandKind.Uninstall => Ui.Button("Uninstall…", () => _ = UninstallCatalog(entry, source, line.Target)),
            _ => throw new ArgumentOutOfRangeException(nameof(command), command, null)
        };
    }

    private FrameworkElement CatalogJobCommand(CatalogService entry, CatalogJob job, ServiceRowCommandKind command)
    {
        var target = new ServiceReleaseTarget(entry.Id, ServiceReleaseSource.Catalog, job.Line, entry.Name, job.Version);
        return command switch
        {
            ServiceRowCommandKind.ShowLog => Ui.Link("Show Log", () => _ = ShowCatalogLog(entry)),
            ServiceRowCommandKind.Cancel => Ui.Button("Cancel", () => _ = RunCatalogAsync(entry.Id, () => Model.ReleaseActions.RunAsync(target, command))),
            ServiceRowCommandKind.Retry => Ui.Button("Try Again", () => _ = RunCatalogAsync(entry.Id, () => Model.ReleaseActions.RunAsync(target, command)),
                true, Ui.GlyphRefresh),
            ServiceRowCommandKind.Dismiss => Ui.Button("Dismiss", () => _ = RunCatalogAsync(entry.Id, () => Model.ReleaseActions.RunAsync(target, command))),
            _ => throw new ArgumentOutOfRangeException(nameof(command), command, null)
        };
    }

    private async Task SwitchCatalog(CatalogService entry, CatalogLine target, ServiceReleaseTarget releaseTarget)
    {
        var own = target.HasData ? $"{entry.Name} {target.Label} uses its own data." : $"{entry.Name} {target.Label} starts with empty data.";
        if (!entry.IsInstalled || await Window.Confirm($"Switch to {entry.Name} {target.Label}?",
                $"{entry.Name} {entry.Line} stops; its data stays but isn't used until you switch back. {own}", "Switch"))
        {
            await RunCatalogAsync(entry.Id, () => Model.ReleaseActions.RunAsync(releaseTarget, ServiceRowCommandKind.Activate));
        }
    }

    private async Task UninstallCatalog(CatalogService entry, CatalogLine line, ServiceReleaseTarget releaseTarget)
    {
        if (line.Custom)
        {
            var successor = entry.Lines.FirstOrDefault(l => l.Installed && l.Id != line.Id);
            var effect = !line.Active ? ""
                : successor is not null ? $"{entry.Name} {line.Label} stops and {entry.Name} {successor.Label} becomes the active version. "
                : $"{entry.Name} {line.Label} stops; your sites can't use {entry.Name} until you start a version again. ";
            if (await Window.Confirm($"Remove {entry.Name} {line.Label}?",
                    effect + $"Flock stops using it. The base directory {line.BaseDir} and the data directory {line.DataDir} stay exactly as they are; Flock never deletes folders it didn't create.", "Remove", true))
            {
                await RunCatalogAsync(entry.Id, () => Model.ReleaseActions.RunAsync(releaseTarget, ServiceRowCommandKind.Uninstall, false));
            }
            return;
        }
        var next = entry.Lines.FirstOrDefault(l => l.Installed && l.Id != line.Id);
        var details = !line.Active ? ""
            : next is not null ? $"{entry.Name} {line.Label} stops and {entry.Name} {next.Label} becomes the active version. "
            : $"{entry.Name} {line.Label} stops; your sites can't use {entry.Name} until you install a version again. ";
        details += line.HasData
            ? $"Its data stays unless you delete it below; installing {line.Label} again picks it up."
            : "It holds no data.";

        CheckBox? delete = null;
        UIElement? extra = null;
        if (line.HasData)
        {
            delete = new CheckBox { Content = "Also delete its data" };
            var warning = Ui.Caption("Deleted data can't be recovered.", Ui.Caution);
            warning.Visibility = Visibility.Collapsed;
            warning.Margin = new Thickness(28, -4, 0, 0);
            delete.Checked += (_, _) => warning.Visibility = Visibility.Visible;
            delete.Unchecked += (_, _) => warning.Visibility = Visibility.Collapsed;
            extra = Ui.Column(4, delete, warning);
        }
        if (await Window.Confirm($"Uninstall {entry.Name} {line.Label}?", details, "Uninstall", true, extra))
        {
            var deleteData = delete?.IsChecked == true;
            await RunCatalogAsync(entry.Id, () => Model.ReleaseActions.RunAsync(releaseTarget, ServiceRowCommandKind.Uninstall, deleteData));
        }
    }

    private async Task RunCatalogAsync(string id, Func<Task<OperationOutcome>> action)
    {
        Kick();
        await action();
        Kick();
    }

    private Task ShowCatalogLog(CatalogService entry)
    {
        return JobLogDialog.ShowAsync(Window, $"{entry.Name} install log", token => Model.Catalog.ReadLogAsync(entry.Id, token), () => CatalogWorking(entry.Id));
    }
}