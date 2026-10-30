param([Parameter(Mandatory=$true)][string]$Pdf, [Parameter(Mandatory=$true)][string]$Png, [int]$Width = 1700)
# Rende la prima pagina di un PDF in un PNG con Windows.Data.Pdf (nessuna dipendenza esterna).
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Runtime.WindowsRuntime
$null = [Windows.Storage.StorageFile, Windows.Storage, ContentType = WindowsRuntime]
$null = [Windows.Storage.Streams.InMemoryRandomAccessStream, Windows.Storage.Streams, ContentType = WindowsRuntime]
$null = [Windows.Data.Pdf.PdfDocument, Windows.Data.Pdf, ContentType = WindowsRuntime]
$null = [Windows.Data.Pdf.PdfPageRenderOptions, Windows.Data.Pdf, ContentType = WindowsRuntime]
$asTask = ([System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
    $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation`1' })[0]
$asAction = ([System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
    $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncAction' })[0]
function Await($op, [Type]$type) { $t = $asTask.MakeGenericMethod($type).Invoke($null, @($op)); [void]$t.Wait(-1); $t.Result }
function AwaitAction($op) { $t = $asAction.Invoke($null, @($op)); [void]$t.Wait(-1) }
$file = Await ([Windows.Storage.StorageFile]::GetFileFromPathAsync($Pdf)) ([Windows.Storage.StorageFile])
$doc = Await ([Windows.Data.Pdf.PdfDocument]::LoadFromFileAsync($file)) ([Windows.Data.Pdf.PdfDocument])
$page = $doc.GetPage(0)
$opt = New-Object Windows.Data.Pdf.PdfPageRenderOptions
$opt.DestinationWidth = [uint32]$Width
$ms = New-Object Windows.Storage.Streams.InMemoryRandomAccessStream
AwaitAction ($page.RenderToStreamAsync($ms, $opt))
$ms.Seek(0)
$bytes = New-Object byte[] ([int]$ms.Size)
$reader = [System.IO.WindowsRuntimeStreamExtensions]::AsStreamForRead($ms)
$n = 0
while ($n -lt $bytes.Length) { $r = $reader.Read($bytes, $n, $bytes.Length - $n); if ($r -le 0) { break }; $n += $r }
[System.IO.File]::WriteAllBytes($Png, $bytes)
"ok"
