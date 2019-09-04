// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"fmt"
	"github.com/gabriel-vasile/mimetype"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/appengine"
	"google.golang.org/appengine/file"
	"google.golang.org/appengine/log"
)

var LEGTHOFFILENAME = 10

func randomString() string {
	const letterBytes = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

	b := make([]byte, LEGTHOFFILENAME)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}
func main() {
	rand.Seed(time.Now().UnixNano())
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/b/", binHandler)
	http.HandleFunc("/upload", uploadHandler)
	appengine.Main()
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	fmt.Fprint(w, `<!doctype html> <html> <head> <title>`+r.URL.Host+`</title>
	</head>
	<body style="background-color:black;color:#ccc">
	<center>
	<h1> Hello There!</h1>
	<p>Usage:</p>
	<pre>This service allows you to store files only 1 day.</pre>
	<pre>You can use two different command to send your file</pre>
	<pre>You can use pipe to redirect your <command> (such as ls, whoami, ps) output to curl</pre>
	<code style="color:#00FF00">command | curl -F 'file=@-' https://up10.me/upload</code>
	<pre>Or you can redirect file to curl</pre>
	<code style="color:#00FF00">curl -F 'file=@-' https://up10.me/upload < file.xxx</code>
	<pre>Most of the files can be stored such as .png, .jpg, .gif even .pdf</pre>
	<pre>If you want more filetype please contact us</pre>
	<a href="https://twitter.com/0xF61" style="color:yellow">Emirhan KURT</a> <br>
	<a href="https://twitter.com/mertcangokgoz" style="color:yellow">Mertcan GÖKGÖZ</a>
	</center>
	</body></html>`)

}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/upload" {
		http.NotFound(w, r)
		return
	}

	// Only accept POST Request
	if r.Method == "GET" {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	// Create buffer for store the request
	var Buf bytes.Buffer
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	}

	defer file.Close()
	io.Copy(&Buf, file)
	contents := Buf.String()
	fileName := randomString()
	ext := ""
	_, ext = mimetype.Detect([]byte(contents))

	switch ext {
	case "png", "jpg", "gif", "webp", "bmp", "ico", "svg":
		fmt.Fprint(w, "https://"+r.URL.Host+"/b/"+fileName+"."+ext)
		writeToCloudStorage(r, contents, fileName, ext)
	case "pdf", "txt":
		fmt.Fprint(w, "https://"+r.URL.Host+"/b/"+fileName+"."+ext)
		writeToCloudStorage(r, contents, fileName, ext)
	case "html", "php":
		fmt.Fprint(w, "Wowowow H4x0r.")
	default:
		fmt.Fprint(w, "Please contact us for "+ext+".")
	}

	Buf.Reset()
}

func binHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path[0:3] != "/b/" {
		http.NotFound(w, r)
		return
	}
	// Only accept GET Request
	if r.Method == "POST" {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}
	fileName := strings.Split(r.URL.Path, "/")[2]
	readFromCloudStorage(r, w, fileName)
}

func writeToCloudStorage(r *http.Request, contents string, fileName string, extension string) error {
	ctx := appengine.NewContext(r)

	// determine default bucket name
	bucketName, err := file.DefaultBucketName(ctx)
	if err != nil {
		log.Errorf(ctx, "failed to get default GCS bucket name: %v", err)
		return err
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		log.Errorf(ctx, "failed to get default GCS bucket name: %v", err)
		return err
	}
	defer client.Close()

	bucket := client.Bucket(bucketName)
	wc := bucket.Object(fileName + "." + extension).NewWriter(ctx)
	wc.ContentType, extension = mimetype.Detect([]byte(contents))

	if _, err := wc.Write([]byte(contents)); err != nil {
		log.Errorf(ctx, "createFile: unable to write data to bucket %q, file %q: %v", bucket, fileName, err)
		return err
	}
	if err := wc.Close(); err != nil {
		log.Errorf(ctx, "createFile: unable to close bucket %q, file %q: %v", bucket, fileName, err)
		return err
	}
	return nil
}

func readFromCloudStorage(r *http.Request, w http.ResponseWriter, fileName string) error {
	ctx := appengine.NewContext(r)

	// determine default bucket name
	bucketName, err := file.DefaultBucketName(ctx)
	if err != nil {
		log.Errorf(ctx, "failed to get default GCS bucket name: %v", err)
		return err
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		log.Errorf(ctx, "failed to get default GCS bucket name: %v", err)
		return err
	}
	defer client.Close()

	bucket := client.Bucket(bucketName)
	rc, err := bucket.Object(fileName).NewReader(ctx)
	if err != nil {
		return err
	}

	defer rc.Close()
	slurp, err := ioutil.ReadAll(rc)

	mime := ""
	mime, _ = mimetype.Detect([]byte(slurp))
	w.Header().Add("Content-Type", mime)
	fmt.Fprintf(w, "%s\n", slurp)

	return nil
}
