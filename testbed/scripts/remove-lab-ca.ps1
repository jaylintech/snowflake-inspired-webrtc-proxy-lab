$certs = Get-ChildItem "Cert:\CurrentUser\CA"
foreach ($c in $certs) {
    if ($c.Subject -like "*Snowflake*") {
        Write-Host "Found: $($c.Thumbprint) - $($c.Subject)"
        $c | Remove-Item
        Write-Host "Removed."
    }
}
if (-not ($certs | Where-Object { $_.Subject -like "*Snowflake*" })) {
    Write-Host "No Snowflake lab CA found in CurrentUser\CA - nothing to remove."
}
