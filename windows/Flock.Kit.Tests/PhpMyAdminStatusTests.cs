namespace Flock.Kit.Tests;

public class PhpMyAdminStatusTests
{
    [Fact]
    public void TheStatusCarriesTheInstalledPhpMyAdminForThePanel()
    {
        var status = FlockJson.Decode<Status>("""
            {"version":"1.0","phpmyadmin":{"version":"5.2.3","domain":"phpmyadmin.test","url":"http://phpmyadmin.test"}}
            """);

        Assert.Equal("5.2.3", status.PhpMyAdmin!.Version);
        Assert.Equal("phpmyadmin.test", status.PhpMyAdmin.Domain);
        Assert.Equal("http://phpmyadmin.test", status.PhpMyAdmin.Url);
        Assert.Null(FlockJson.Decode<Status>("""{"version":"1.0"}""").PhpMyAdmin);
    }
}
