package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/gabriel-vasile/mimetype"
	"github.com/lithammer/shortuuid"
	"google.golang.org/appengine"
)

type upfile struct {
	name     string
	origName string
	ext      string
	content  []byte
}

var (
	storageClient *storage.Client
	// Set this in app.yaml when running in production.
	bucketName = os.Getenv("GCLOUD_STORAGE_BUCKET")
)

func (u *upfile) URL(r *http.Request) string {
	return fmt.Sprintf("https://%s/b/%s%s", r.Host, u.name, u.ext)
}
func (u *upfile) FileName() string {
	return fmt.Sprintf("%s%s", u.name, u.ext)
}

func main() {
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/b/", binHandler)
	http.HandleFunc("/upload", uploadHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
		log.Printf("Defaulting to port %s", port)
	}

	log.Printf("Listening on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// They don't have to post only /upload path.
	if r.Method == "POST" {
		uploadHandler(w, r)
		return
	}

	fmt.Fprint(w, `<!doctype html> <html> <head> <title>`+r.Host+`</title>
	<link rel="shortcut icon" type="image/png" href="/s/favicon.png"/>
	<!-- Latest Deploy: Wed 05 Feb 2020 07:12:27 PM +03 -->
	</head>
	<body style="background-color:black;color:#ccc">
	<center>
	<h1> Hello There!</h1>
	<pre>This service allows you to store files only 1 day.</pre>
	<b>Usage:</b>
	<pre>You can use two different command to send your file</pre>
	<pre>You can either use pipe to redirect your command (such as ls, whoami, ps) output to curl</pre>
	<code style="color:#00FF00">command | curl -F 'file=@-' https://`+r.Host+`/</code>
	<pre>Or you can redirect file to curl</pre>
	<code style="color:#00FF00">curl -F 'file=@-' https://`+r.Host+`/ < file.xxx</code>
	<pre>Most of the files can be stored such as .png, .jpg, .gif even .pdf</pre>
	<h3>If you use ShareX you can use these configs</h3>
	<a href="https://getsharex.com/" style="color:yellow">You can get ShareX here</a> <br>
	<a href="/s/up10.sxcu" style="color:yellow">Image configuration</a> <br>
	<a href="/s/up10-file.sxcu" style="color:yellow">File configuration</a> <br>
	<h3>If you want more filetype please contact us</h3>
	<a href="https://twitter.com/0xF61" style="color:yellow">Emirhan KURT</a> <br>
	<a href="https://twitter.com/mertcangokgoz" style="color:yellow">Mertcan GÖKGÖZ</a>
	<h3>Or if you antisocial you can directly offer us to PR.</h3>
	<a href="https://github.com/foss-dev/up10.me" style="color:yellow">Github</a>
	</center>
	</body></html>`)

}

func uploadHandler(w http.ResponseWriter, r *http.Request) {

	// Only accept POST Request
	if r.Method == "GET" {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	formfile, fileHeader, err := r.FormFile("file")
	if err != nil {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}
	defer formfile.Close()

	ufile := upfile{}
	ufile.name = shortuuid.New()
	ufile.content, err = ioutil.ReadAll(formfile)
	if fileHeader.Filename == "-" {
		ufile.origName = ufile.name
	} else {
		ufile.origName = fileHeader.Filename
	}

	if err != nil {
		fmt.Fprint(w, err)
		return
	}

	ufile.ext = mimetype.Detect(ufile.content).Extension()

	switch ufile.ext {
	case "exe", "jar", "deb", "xlf", "": // We don't want to allow this ext
		fmt.Fprint(w, fmt.Sprintf("Please contact us for %s", ufile.ext))
	default:
		if err := writeToCloudStorage(w, r, &ufile); err != nil {
			fmt.Fprint(w, err)
			return
		}
		fmt.Fprint(w, ufile.URL(r), "\n")
	}
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

func writeToCloudStorage(w http.ResponseWriter, r *http.Request, ufile *upfile) error {
	ctx := appengine.NewContext(r)

	client, err := storage.NewClient(ctx)
	if err != nil {
		log.Println(ctx, "failed to get default GCS bucket name: %v", err)
		return err
	}
	defer client.Close()

	bucket := client.Bucket(bucketName)
	wc := bucket.Object(ufile.FileName()).NewWriter(ctx)
	wc.ContentDisposition = ufile.origName // + "." + ufile.ext   not sure here

	size, err := wc.Write(ufile.content)
	if err != nil {
		log.Println(ctx, "createFile: unable to write bucket %q, file: %s Size:%d, %v", bucket, ufile.FileName(), size, err)
		return err
	}

	if err := wc.Close(); err != nil {
		log.Println(ctx, "createFile: unable to close bucket %q, file %q: %v", bucket, ufile.FileName(), err)
		return err
	}
	return nil

}
func readFromCloudStorage(r *http.Request, w http.ResponseWriter, fileName string) error {
	ctx := appengine.NewContext(r)

	client, _ := storage.NewClient(ctx)
	defer client.Close()

	bucket := client.Bucket(bucketName)
	bucketObject := bucket.Object(fileName)
	rc, err := bucketObject.NewReader(ctx)
	if err != nil {
		fmt.Println(err)
	}
	defer rc.Close()

	slurp, err := ioutil.ReadAll(rc)
	if err != nil {
		fmt.Fprint(w, err)
	}
	ext := mimetype.Detect(slurp)

	// Grab ContentDisposition
	o, _ := bucketObject.Attrs(ctx)
	CD := o.ContentDisposition

	// It can be shortuuid but nothing wrong about it
	w.Header().Add("Content-Disposition", "filename=\""+string(CD)+"\"")

	switch ext.Extension() {
	case ".html", ".py", ".js", ".wasm":
		w.Header().Add("Content-Type", "text/plain")
	default:
		w.Header().Add("Content-Type", ext.Parent().String())
	}
	fmt.Fprintf(w, "%s", slurp)
	return nil
}
