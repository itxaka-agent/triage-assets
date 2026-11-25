// QA rogue seeder: reverse proxy to a real seeder; corrupts bytes of image range responses.
package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
)

func main() {
	up := flag.String("up", "http://10.77.0.1:42069", "")
	listen := flag.String("listen", "10.77.0.3:42069", "")
	mode := flag.String("mode", "flip", "flip|slow")
	flag.Parse()
	var n int64
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		req, _ := http.NewRequest(r.Method, *up+r.URL.RequestURI(), nil)
		if rg := r.Header.Get("Range"); rg != "" {
			req.Header.Set("Range", rg)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if *mode == "flip" && len(b) > 1000000 {
			c := atomic.AddInt64(&n, 1)
			b[len(b)/2] ^= 0xff
			fmt.Printf("corrupted response #%d %s %s len=%d\n", c, r.URL.Path, r.Header.Get("Range"), len(b))
		}
		for k, v := range resp.Header {
			if k != "Content-Length" {
				w.Header()[k] = v
			}
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(b)))
		w.WriteHeader(resp.StatusCode)
		w.Write(b)
	})
	fmt.Println("rogue on", *listen)
	http.ListenAndServe(*listen, nil)
}
