<?php
// LibreSpeed - Empty endpoint
// 1) Used as ping target (HEAD/GET, returns immediately)
// 2) Used as upload sink (POST, discards data)

header('HTTP/1.1 200 OK');
header('Content-Length: 0');
header('Content-Type: text/plain');
header('Connection: keep-alive');
header('Cache-Control: no-store, no-cache, must-revalidate, max-age=0');
header('Cache-Control: post-check=0, pre-check=0', false);
header('Pragma: no-cache');
// Mitigate CORS issues if hit cross-origin
header('Access-Control-Allow-Origin: *');
header('Access-Control-Allow-Methods: GET, POST, HEAD, OPTIONS');

// For POST: read and discard the body to ensure full TCP receive
if ($_SERVER['REQUEST_METHOD'] === 'POST') {
    $stream = fopen('php://input', 'rb');
    if ($stream) {
        while (!feof($stream)) {
            fread($stream, 1024 * 64);
        }
        fclose($stream);
    }
}
exit;
