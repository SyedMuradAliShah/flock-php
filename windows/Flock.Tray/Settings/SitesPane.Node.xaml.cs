using Flock.Kit;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
namespace Flock.Tray.Settings;

/// <summary>
///     What Flock runs for a site or proxy: a Node.js site's dev server (started on the site's first
///     visit, asleep after 30 idle minutes) and a proxy's start command, each with its state, Start
///     and Stop, and its output.
/// </summary>
internal sealed partial class SitesPane
{
    private const string CommandHelp =
        "Flock runs the command when you click Start and shows its output here. For a command that starts the server in the background, like docker compose up -d, add the command that stops it.";

    /// <summary>The site's type: PHP, or a Node.js app whose dev server Flock runs.</summary>
    private ComboBox TypePicker(Status.Site site)
    {
        var node = site.IsNode;
        var type = new ComboBox { MinWidth = 140 };
        type.Items.Add(new ComboBoxItem { Content = "PHP" });
        type.Items.Add(new ComboBoxItem { Content = "Node.js" });
        type.SelectedIndex = node ? 1 : 0;
        type.SelectionChanged += (_, _) =>
        {
            if (type.SelectedIndex < 0 || type.SelectedIndex == 1 == node)
            {
                return;
            }
            var change = type.SelectedIndex == 1 ? Status.SiteType.Node : Status.SiteType.Php;
            Start(site.Name, () => UpdateSiteAsync(site.Name, new SiteChange(Type: change), null), false);
        };
        Ui.AutomationName(type, "Type");
        return type;
    }

    /// <summary>A Node.js site's dev server: its state, Start, Stop and Restart, the command and its output.</summary>
    private StackPanel DevServer(Status.Site site)
    {
        var section = Section("Dev server");
        if (site.NodeServer is not { } server)
        {
            section.Children.Add(Ui.Caption("Flock starts the dev server on the site's first visit.", Ui.TertiaryText));
            return section;
        }
        var tone = server.State switch
        {
            Status.NodeServerState.Running => Ui.Tone.Success,
            Status.NodeServerState.Starting => Ui.Tone.Caution,
            Status.NodeServerState.Failed => Ui.Tone.Critical,
            _ => Ui.Tone.Neutral,
        };
        section.Children.Add(Field("State", StateLine(tone, server.StateLabel,
            server.State != Status.NodeServerState.Failed ? server.Problem : "")));
        var command = Ui.MonoText(server.Command, 12);
        command.Foreground = Ui.SecondaryText;
        section.Children.Add(Field("Command", command));

        var buttons = Ui.Row(8);
        if (server.CanStart)
        {
            buttons.Children.Add(Ui.Button("Start", () => Start(site.Name, () => Model.Sites.RestartAsync(site.Name))));
        }
        if (server.CanStop)
        {
            buttons.Children.Add(Ui.Button("Stop", () => Start(site.Name, () => Model.Sites.StopSiteAsync(site.Name))));
        }
        if (server.CanRestart)
        {
            buttons.Children.Add(Ui.Button("Restart", () => Start(site.Name, () => Restart(site.Name))));
        }
        section.Children.Add(buttons);
        Output(section, server.Log, server.State is Status.NodeServerState.Starting or Status.NodeServerState.Running);
        section.Children.Add(Ui.Caption(
            "Flock runs the project's dev script with its Node.js version when the site is visited, and stops it after 30 minutes without visits.",
            Ui.TertiaryText));
        return section;
    }

    /// <summary>A proxy's start command: its state, Start and Stop, the command and its output; or a way to add one.</summary>
    private StackPanel ProxyCommandSection(Status.Proxy proxy)
    {
        var section = Section("Start command");
        if (proxy.Command is not { } command)
        {
            section.Children.Add(Ui.Button("Add Command…", () => Start(proxy.Name, () => ChangeCommandAsync(proxy))));
            section.Children.Add(Ui.Caption(CommandHelp, Ui.TertiaryText));
            return section;
        }
        var tone = command.State switch
        {
            Status.ProxyState.Running => Ui.Tone.Success,
            Status.ProxyState.Starting or Status.ProxyState.Stopping => Ui.Tone.Caution,
            Status.ProxyState.Failed => Ui.Tone.Critical,
            _ => Ui.Tone.Neutral,
        };
        section.Children.Add(Field("State", StateLine(tone, command.StateLabel,
            command.State == Status.ProxyState.Starting ? command.Problem : "")));
        section.Children.Add(Field("Command", Ui.MonoText(command.Command, 12)));
        var folder = Ui.Text(command.Dir, wrap: false);
        ToolTipService.SetToolTip(folder, command.Dir);
        section.Children.Add(Field("Folder", folder));
        if (command.StopCommand.Length > 0)
        {
            section.Children.Add(Field("Stop command", Ui.MonoText(command.StopCommand, 12)));
        }

        var buttons = Ui.Row(8);
        if (command.CanStart)
        {
            buttons.Children.Add(Ui.Button("Start", () => Start(proxy.Name, () => Model.Sites.StartProxyAsync(proxy.Name))));
        }
        if (command.CanStop)
        {
            buttons.Children.Add(Ui.Button("Stop", () => Start(proxy.Name, () => Model.Sites.StopProxyAsync(proxy.Name))));
        }
        buttons.Children.Add(Ui.Button("Change…", () => Start(proxy.Name, () => ChangeCommandAsync(proxy))));
        section.Children.Add(buttons);
        Output(section, command.Log,
            command.State is Status.ProxyState.Starting or Status.ProxyState.Running or Status.ProxyState.Stopping);
        section.Children.Add(Ui.Caption(CommandHelp, Ui.TertiaryText));
        return section;
    }

    /// <summary>A state dot with its text, and a second line saying what's wrong (problem may be empty).</summary>
    private static StackPanel StateLine(Ui.Tone tone, string text, string problem)
    {
        var line = Ui.Column(2, Ui.Row(6, Ui.StateDot(tone), Ui.Text(text)));
        if (problem.Length > 0)
        {
            line.Children.Add(Ui.Caption(problem, Ui.Caution));
        }
        return line;
    }

    /// <summary>Puts the output panel for log into section, read every second while live; one panel moves between the details.</summary>
    private void Output(Panel section, string log, bool live)
    {
        outputShown = true;
        output ??= new OutputPanel(Model.Sites);
        output.ShowIn(section, log, live && active);
    }

    /// <summary>Asks for a proxy's command, its folder and its stop command; an empty command removes them.</summary>
    private async Task ChangeCommandAsync(Status.Proxy proxy)
    {
        var current = proxy.Command;
        var command = new TextBox { Header = "Command", PlaceholderText = "npm run dev", Text = current?.Command ?? "", FontFamily = Ui.Mono };
        var folder = new TextBox
        {
            Text = current?.Dir is { Length: > 0 } dir ? dir : Environment.GetFolderPath(Environment.SpecialFolder.UserProfile),
            MinWidth = 280,
        };
        var choose = Ui.Button("Choose…", () => _ = ChooseFolderAsync());
        async Task ChooseFolderAsync()
        {
            if (await Window.PickFolder() is { } picked)
            {
                folder.Text = picked;
            }
        }
        var stop = new TextBox
        {
            Header = "Stop command", PlaceholderText = "Only for a server that runs in the background, e.g. docker compose stop",
            Text = current?.StopCommand ?? "", FontFamily = Ui.Mono,
        };
        var dialog = new ContentDialog
        {
            Title = current is null ? $"Add a start command to {proxy.Name}" : $"Change the start command of {proxy.Name}",
            Content = Ui.Column(12, Ui.Text(CommandHelp), command,
                Ui.Column(4, Ui.Text("Folder"), Ui.Row(8, folder, choose)), stop),
            PrimaryButtonText = "Save",
            SecondaryButtonText = current is null ? "" : "Remove Command",
            CloseButtonText = "Cancel",
            DefaultButton = ContentDialogButton.Primary,
            XamlRoot = Window.XamlRoot,
        };
        var result = await dialog.ShowAsync();
        ProxyChange change;
        switch (result)
        {
            case ContentDialogResult.Secondary:
                change = new ProxyChange(Command: "");
                break;
            case ContentDialogResult.Primary:
                var line = command.Text.Trim();
                change = line.Length == 0
                    ? new ProxyChange(Command: "")
                    : new ProxyChange(Command: line, Dir: folder.Text.Trim(), StopCommand: stop.Text.Trim());
                break;
            default:
                return;
        }
        if (current is null && change.Command == "")
        {
            return;
        }
        await Model.Sites.UpdateProxyAsync(proxy.Name, change);
    }
}
