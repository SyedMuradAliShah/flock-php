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

public class RootPasswordTests
{
    [Fact]
    public void ACatalogEntryKnowsWhetherRootHasAPassword()
    {
        var catalog = FlockJson.Decode<ServiceCatalog>("""
            {"services":[{"id":"mariadb","name":"MariaDB","installed":"12.3.3","root_password":true},
                         {"id":"mysql","name":"MySQL","installed":"8.4.1"},
                         {"id":"valkey","name":"Valkey","installed":"9.1.3"}]}
            """);

        Assert.True(catalog["mariadb"]!.RootPassword);
        Assert.False(catalog["mysql"]!.RootPassword);
        Assert.True(catalog["mariadb"]!.HasRootUser);
        Assert.True(catalog["mysql"]!.HasRootUser);
        Assert.False(catalog["valkey"]!.HasRootUser);
    }
}

public class RootPasswordPerLineTests
{
    [Fact]
    public void EveryLineSaysWhetherItHasItsOwnRootPassword()
    {
        var entry = FlockJson.Decode<ServiceCatalog>("""
            {"services":[{"id":"mariadb","name":"MariaDB","installed":"12.3.3","line":"12.3","root_password":true,
              "lines":[{"id":"12.3","installed":true,"active":true,"root_password":true},
                       {"id":"11.8","installed":true,"root_password":false},
                       {"id":"10.11","installed":true,"root_password":true}]}]}
            """)["mariadb"]!;

        Assert.Equal([true, false, true], entry.Lines.Select(l => l.RootPassword));
        Assert.Equal(3, entry.Lines.Count(l => l.Installed));
    }
}
