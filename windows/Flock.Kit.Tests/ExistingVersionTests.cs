namespace Flock.Kit.Tests;

public class ExistingVersionTests
{
    [Fact]
    public void ALineKnowsItsFoldersAndWhetherItIsTheUsersOwn()
    {
        var entry = FlockJson.Decode<ServiceCatalog>("""
            {"services":[{"id":"mariadb","name":"MariaDB","installed":"10.11.2","line":"custom-10.11.2",
              "lines":[{"id":"custom-10.11.2","label":"10.11.2 (custom)","version":"10.11.2","installed":true,"active":true,"custom":true,
                        "basedir":"C:\\tools\\mariadb-10.11.2","datadir":"C:\\data\\mariadb","has_data":true},
                       {"id":"12.3","version":"12.3.3","installed":true,"basedir":"C:\\Flock\\services\\mariadb\\12.3.3","datadir":"C:\\Flock\\services\\mariadb\\data\\12.3"},
                       {"id":"11.8","installed":false}]}]}
            """)["mariadb"]!;

        var own = entry.Lines[0];
        Assert.True(own.Custom);
        Assert.Equal("10.11.2 (custom)", own.Label);
        Assert.Equal(@"C:\tools\mariadb-10.11.2", own.BaseDir);
        Assert.Equal(@"C:\data\mariadb", own.DataDir);

        Assert.False(entry.Lines[1].Custom);
        Assert.Equal(@"C:\Flock\services\mariadb\data\12.3", entry.Lines[1].DataDir);
        Assert.Equal("", entry.Lines[2].BaseDir);
        Assert.Equal("", entry.Lines[2].DataDir);
    }

    [Fact]
    public void OnlyMySqlAndMariaDbAcceptAnExistingVersion()
    {
        Assert.True(new CatalogService { Id = "mysql" }.AcceptsExisting);
        Assert.True(new CatalogService { Id = "mariadb" }.AcceptsExisting);
        Assert.False(new CatalogService { Id = "valkey" }.AcceptsExisting);
    }
}

public class MissingExistingVersionTests
{
    [Fact]
    public void AVersionWhoseFolderIsGoneIsMarkedSoItCanBeRemoved()
    {
        var line = FlockJson.Decode<ServiceCatalog>("""
            {"services":[{"id":"mariadb","lines":[{"id":"custom-10.11.2","installed":true,"custom":true,"missing":true}]}]}
            """)["mariadb"]!.Lines[0];

        Assert.True(line.Custom);
        Assert.True(line.Missing);
        Assert.True(line.Installed);
    }
}
