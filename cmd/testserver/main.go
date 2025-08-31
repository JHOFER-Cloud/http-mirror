package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

func main() {
	// Create a simple file server that mimics Apache/Nginx directory listings
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		
		switch r.URL.Path {
		case "/":
			// Serve directory listing
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			html := `<!DOCTYPE html>
<html>
<head>
    <title>Index of /</title>
</head>
<body>
    <h1>Index of /</h1>
    <hr>
    <pre>
<a href="file1.txt">file1.txt</a>                               ` + time.Now().Format("2006-01-02 15:04") + `     27
<a href="file2.txt">file2.txt</a>                               ` + time.Now().Format("2006-01-02 15:04") + `     42
<a href="subdir/">subdir/</a>                                   ` + time.Now().Format("2006-01-02 15:04") + `      -
    </pre>
    <hr>
    <address>HTTP Mirror Test Server</address>
</body>
</html>`
			w.Write([]byte(html))

		case "/file1.txt":
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Last-Modified", time.Now().Format(time.RFC1123))
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("This is the content of file1.txt!\nIt has some test content.\n"))

		case "/file2.txt":
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Last-Modified", time.Now().Format(time.RFC1123))
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("This is file2.txt with different content.\nMore lines here.\nAnd even more content!\n"))

		case "/subdir/":
			// Serve subdirectory listing
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			html := `<!DOCTYPE html>
<html>
<head>
    <title>Index of /subdir/</title>
</head>
<body>
    <h1>Index of /subdir/</h1>
    <hr>
    <pre>
<a href="../">../</a>                                          ` + time.Now().Format("2006-01-02 15:04") + `      -
<a href="nested.txt">nested.txt</a>                             ` + time.Now().Format("2006-01-02 15:04") + `     35
<a href="another.log">another.log</a>                           ` + time.Now().Format("2006-01-02 15:04") + `    128
    </pre>
    <hr>
    <address>HTTP Mirror Test Server</address>
</body>
</html>`
			w.Write([]byte(html))

		case "/subdir/nested.txt":
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Last-Modified", time.Now().Format(time.RFC1123))
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("This is nested.txt inside subdir.\nNested content here!\n"))

		case "/subdir/another.log":
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Last-Modified", time.Now().Format(time.RFC1123))
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Log file content:\n2024-08-31 12:00:00 INFO: Server started\n2024-08-31 12:01:15 DEBUG: Processing request\n2024-08-31 12:02:30 INFO: Request completed\n"))

		default:
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("404 Not Found"))
		}
	})

	port := "8080"
	fmt.Printf("🚀 Test server starting on http://localhost:%s\n", port)
	fmt.Printf("📁 Directory listing available at: http://localhost:%s/\n", port)
	fmt.Printf("📄 Direct files:\n")
	fmt.Printf("   - http://localhost:%s/file1.txt\n", port)
	fmt.Printf("   - http://localhost:%s/file2.txt\n", port)
	fmt.Printf("   - http://localhost:%s/subdir/\n", port)
	fmt.Printf("   - http://localhost:%s/subdir/nested.txt\n", port)
	fmt.Printf("   - http://localhost:%s/subdir/another.log\n", port)
	fmt.Printf("\n🔄 Use Ctrl+C to stop the server\n")

	log.Fatal(http.ListenAndServe(":"+port, nil))
}