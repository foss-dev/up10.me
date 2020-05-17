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

var tmpHTMLPage []byte

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
	var err error
	tmpHTMLPage, err = ioutil.ReadFile("s/index.html")
	if err != nil {
		log.Println("Template file couldn't found")
	}

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

	htmlPage := strings.ReplaceAll(string(tmpHTMLPage), "{{Host}}", r.Host)
	fmt.Fprint(w, htmlPage)
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

	if ufile.ext == "" {
		ufile.ext = "."
		if strings.Contains(ufile.origName, ".") {
			s := strings.Split(ufile.origName, ".")
			ufile.ext += s[len(s)-1]
		} else {
			ufile.ext += "up10"
		}
	}
	switch ufile.ext {
	case "exe", "jar", "deb", "xlf": // We don't want to allow this ext
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
	//log.Println("Creating Background")
	//ctx := context.Background()

	ctx := appengine.NewContext(r)

	client, err := storage.NewClient(ctx)
	if err != nil {
		log.Printf("Failed to Get Default GCS bucket name: %v", ctx)
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
		log.Println(err)
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
		w.Header().Add("Content-Type", ext.String())
	}
	fmt.Fprintf(w, "%s", slurp)
	return nil
}
