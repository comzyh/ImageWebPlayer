package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/gorilla/mux"
	"github.com/kjk/lzmadec"
)

type ImageServer struct {
	rootdir  string
	archives map[string]interface{}
}

func (s *ImageServer) openArchive(path string) (archive *lzmadec.Archive, err error) {
	archive, ok := s.archives[path].(*lzmadec.Archive)
	if !ok {
		archive, err = lzmadec.NewArchive(path)
		s.archives[path] = archive
	}
	return
}
func (s *ImageServer) archive_file(w http.ResponseWriter, r *http.Request) {
	deode_uri, _ := url.PathUnescape(r.RequestURI[len("/archive_file/"):])
	splitPath := strings.Split(deode_uri, "//")
	archivePath := filepath.Join(s.rootdir, splitPath[0])
	pathInArchive := splitPath[1]

	archive, err := s.openArchive(archivePath)
	// archive, err := lzmadec.NewArchive(archivePath)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	reader, err := archive.GetFileReader(pathInArchive)
	defer reader.Close()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", mime.TypeByExtension(filepath.Ext(pathInArchive)))
	io.Copy(w, reader)
}

func (s *ImageServer) getArchiveFilesAndDir(path string) (files []string, dirs []string, err error) {
	splitPath := strings.Split(path, "//")
	archivePath := filepath.Join(s.rootdir, splitPath[0])
	pathInArchive := splitPath[1]

	archive, _ := s.openArchive(archivePath)
	// archive, _ := lzmadec.NewArchive(archivePath)

	files = make([]string, 0)
	dirs = make([]string, 0)
	for _, e := range archive.Entries {
		dir, filename := filepath.Split(e.Path)
		if dir == pathInArchive {
			switch e.Attributes {
			case "D":
				dirs = append(dirs, filename)
			case "A":
				files = append(files, filename)
			}
		}
	}
	return
}
func (s *ImageServer) getFSFilesAndDir(path string) (files []string, dirs []string, err error) {
	path = filepath.Join(s.rootdir, path)
	fileinfos, err := ioutil.ReadDir(path)
	if err != nil {
		return
	}
	files = make([]string, 0)
	dirs = make([]string, 0)
	for _, file := range fileinfos {
		if file.IsDir() {
			dirs = append(dirs, file.Name())
		} else {
			files = append(files, file.Name())
		}
	}
	return
}

func (s *ImageServer) list(w http.ResponseWriter, r *http.Request) {
	var path string
	imageOnly := false
	if strings.HasPrefix(r.RequestURI, "/list/") {
		path = r.RequestURI[len("/list/"):]
	} else {
		path = r.RequestURI[len("/listimg/"):]
		imageOnly = true
	}
	path, _ = url.PathUnescape(path)
	var files, dirs []string
	var err error
	if strings.Contains(path, "//") {
		files, dirs, err = s.getArchiveFilesAndDir(path)
	} else {
		files, dirs, err = s.getFSFilesAndDir(path)
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if imageOnly {
		dirs = make([]string, 0)
		imgfiles := make([]string, 0)
		for _, file := range files {
			switch filepath.Ext(file) {
			case ".png", ".jpeg", ".jpg", ".gif", ".bmp":
				imgfiles = append(imgfiles, file)
			}
		}
		files = imgfiles
	}
	resp := map[string]interface{}{
		"files": files,
		"dirs":  dirs,
	}

	json.NewEncoder(w).Encode(resp)
}

func main() {
	var rootdir = flag.String("root", "", "root dir to view")
	var port = flag.Int("port", 8080, "port to listen")
	var bind_address = flag.String("bind", "127.0.0.1", "address to bind")
	flag.Parse()

	var err error
	abs_root_dir, err := filepath.Abs(*rootdir)
	if err != nil {
		log.Fatal(err)
	}
	server := &ImageServer{abs_root_dir, make(map[string]interface{})}
	r := mux.NewRouter().SkipClean(true)

	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})
	r.HandleFunc("/list/{path:.*}", server.list)
	r.HandleFunc("/listimg/{path:.*}", server.list)
	r.HandleFunc("/archive_file/{path:.*}", server.archive_file)

	r.PathPrefix("/files/").Handler(http.StripPrefix("/files/", http.FileServer(http.Dir(abs_root_dir))))
	http.Handle("/", r)
	log.Fatal(http.ListenAndServe(fmt.Sprintf("%s:%d", *bind_address, *port), r))
}
