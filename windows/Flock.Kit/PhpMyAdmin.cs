using System.Text.Json.Serialization;

namespace Flock.Kit;

/// <summary>
/// GET /v1/phpmyadmin — the phpMyAdmin Flock installs from phpmyadmin.net and serves as the site
/// "phpmyadmin". The install is a job in the daemon (poll this while Job is running); the user can
/// pick the folder it lives in.
/// </summary>
public sealed class PhpMyAdminState
{
    public bool Installed { get; init; }
    public string Version { get; init; } = "";
    /// <summary>Where the versions are installed.</summary>
    public string Folder { get; init; } = "";
    /// <summary>Where they go unless a folder is chosen.</summary>
    public string DefaultFolder { get; init; } = "";
    /// <summary>The site's name, "db" for db.test.</summary>
    public string Name { get; init; } = "";
    public string DefaultName { get; init; } = "";
    public string Domain { get; init; } = "";
    /// <summary>The address to open, with the web server's port; empty when not installed.</summary>
    public string Url { get; init; } = "";
    public string Estimate { get; init; } = "";
    /// <summary>The newest release phpmyadmin.net lists; empty until the first check.</summary>
    public string Latest { get; init; } = "";
    /// <summary>Latest is newer than the installed version.</summary>
    public bool Update { get; init; }
    public DateTimeOffset? CheckedAt { get; init; }
    public string CheckError { get; init; } = "";
    public PhpMyAdminJob? Job { get; init; }

    [JsonIgnore]
    public bool IsBusy => Job?.IsRunning == true;

    [JsonIgnore]
    public bool UsesDefaultName => string.Equals(Name, DefaultName, StringComparison.OrdinalIgnoreCase);

    /// <summary>The domain's ending, ".test", for the address box.</summary>
    [JsonIgnore]
    public string Suffix => Domain.IndexOf('.') is var dot and >= 0 ? Domain[dot..] : "";

    [JsonIgnore]
    public bool UsesDefaultFolder => string.Equals(Folder, DefaultFolder, StringComparison.OrdinalIgnoreCase);
}

/// <summary>A running or failed phpMyAdmin install.</summary>
public sealed class PhpMyAdminJob : JobState
{
    public string Version { get; init; } = "";
}
