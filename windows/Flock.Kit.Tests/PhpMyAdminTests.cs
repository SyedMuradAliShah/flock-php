namespace Flock.Kit.Tests;

public class PhpMyAdminTests
{
    [Fact]
    public void DecodesAnInstalledPhpMyAdmin()
    {
        var state = FlockJson.Decode<PhpMyAdminState>("""
            {"installed":true,"version":"5.2.3","folder":"D:\\Tools\\phpmyadmin","default_folder":"C:\\Flock\\phpmyadmin",
             "domain":"phpmyadmin.test","url":"http://phpmyadmin.test","estimate":"about 16 MB download","job":null}
            """);

        Assert.True(state.Installed);
        Assert.Equal("5.2.3", state.Version);
        Assert.Equal("http://phpmyadmin.test", state.Url);
        Assert.False(state.UsesDefaultFolder);
        Assert.False(state.IsBusy);
        Assert.Null(state.Job);
    }

    [Fact]
    public void DecodesARunningAndAFailedJob()
    {
        var running = FlockJson.Decode<PhpMyAdminState>("""
            {"installed":false,"folder":"C:\\Flock\\phpmyadmin","default_folder":"c:\\flock\\PHPMYADMIN",
             "job":{"state":"running","phase":"downloading","progress":0.5,"error":"","started_at":"2026-11-10T20:00:00.123456789-04:00","version":"5.2.3"}}
            """);
        Assert.True(running.IsBusy);
        Assert.True(running.UsesDefaultFolder);
        Assert.Equal("Downloading…", running.Job!.PhaseText);
        Assert.True(running.Job.HasProgress);
        Assert.Equal("5.2.3", running.Job.Version);

        var failed = FlockJson.Decode<PhpMyAdminState>("""{"job":{"state":"failed","phase":"downloading","progress":-1,"error":"checksum mismatch"}}""");
        Assert.True(failed.Job!.IsFailed);
        Assert.False(failed.IsBusy);
        Assert.Equal("checksum mismatch", failed.Job.Error);
    }

    [Fact]
    public void PhpMyAdminIsNotACatalogService()
    {
        var service = new Service { Id = "phpmyadmin", Name = "phpMyAdmin", Category = "Database Client" };
        Assert.True(service.IsPhpMyAdmin);
        Assert.True(service.IsOnRequest);
        Assert.False(service.IsCatalog);
        Assert.False(service.IsDatabase);
        Assert.True(new Service { Id = "mysql" }.IsCatalog);
    }
}
