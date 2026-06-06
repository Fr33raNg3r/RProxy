<?php
// LibreSpeed - Garbage data generator for download speed test
// Returns N MB of random data (default 20 MB)
// Cache headers prevent client/intermediate caching to ensure real network test

header('HTTP/1.1 200 OK');
header('Content-Description: File Transfer');
header('Content-Type: application/octet-stream');
header('Content-Disposition: attachment; filename=random.dat');
header('Content-Transfer-Encoding: binary');
header('Cache-Control: no-store, no-cache, must-revalidate, max-age=0');
header('Cache-Control: post-check=0, pre-check=0', false);
header('Pragma: no-cache');

// Disable PHP output buffering for streaming
@ini_set('zlib.output_compression', '0');
@ini_set('output_buffering', '0');
@ini_set('implicit_flush', '1');
@ob_implicit_flush(true);
while (ob_get_level() > 0) ob_end_flush();

// 'ckSize' = MB count, default 20 MB, max 1024 MB
$ckSize = isset($_GET['ckSize']) ? intval($_GET['ckSize']) : 20;
if ($ckSize > 1024) $ckSize = 1024;
if ($ckSize < 1) $ckSize = 1;

// Generate 1 MB of random data once, send N times
$data = openssl_random_pseudo_bytes(1024 * 1024);
for ($i = 0; $i < $ckSize; $i++) {
    echo $data;
    flush();
}
