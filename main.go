package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

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

	fmt.Fprint(w, `<!DOCTYPE html>
<!-- Latest build `+time.Now().Format(time.UnixDate)+`-->
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>`+r.Host+`</title>
  <link rel="stylesheet" href="/s/main.css">
  <link rel="shortcut icon" type="image/png" href="/s/favicon.png"/>
  <link href="https://fonts.googleapis.com/css2?family=Open+Sans:wght@300;400;700&display=swap" rel="stylesheet">
	<script>

function uploadFile(){
  const file = document.getElementById('file').files[0]
  if(file.size <= 10485760){
  var fd = new FormData();
  fd.append("file", file);

  var xhr = new XMLHttpRequest();
  xhr.open('POST', '/upload', true);

  xhr.upload.onprogress = function(e) {
    if (e.lengthComputable) {
      var percentComplete = (e.loaded / e.total) * 100;
      console.log(percentComplete + '% uploaded');
      document.getElementById('text').innerHTML = Math.round(percentComplete) + '% uploaded';
    }
  };

  xhr.onload = function() {
    if (this.status == 200) {
      document.getElementById('info').innerHTML = this.response.link(this.response);
			var element = document.getElementById("upten");
			element.parentNode.removeChild(element);
    };
  };

  xhr.send(fd);
}
else{
document.getElementById('text').innerHTML = "Dosya boyutu 10 MB'dan büyük olamaz";
}
};
	</script>
</head>
<body>
  <div class="wrapper">
    <a href="/"><img src="/s/up10.png" alt="up10" class="imaj"></a>
      <header class="header"></header>
      <h1>Hello There!</h1>
			<div>
				<p>This service allows you to store files only 1 day.</p>
				<b>Usage:</b>
				<p>You can use two different command to send your file. You can either <br>
					use pipe to redirect your command (such as ls, whoami, ps) output to curl</p>
				<code style="color:red">command | curl -F 'file=@-' https://`+r.Host+`/</code>
				<p>Or you can redirect file to curl</p>
				<code style="color:red">curl -F 'file=@-' https://`+r.Host+`/ < file.xxx</code>
				<p>Most of the files can be stored such as .png, .jpg, .gif even .pdf</p>
				<b>Or you can use traditional way to upload your file</b>
			</div>
					<p id="info"></p>
          <div id="upten">
						<input type="file" name="filename" id="file" onchange="document.getElementById('text').innerHTML = document.getElementById('file').files[0].name; ">

						<p id="text">Drag your file here or click in this area.</p>
            <button id="buttonid" onclick="uploadFile()">Upload</button>
          </div>
		</div>
</body>
</html>`)
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
