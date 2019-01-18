package main

import (
	"archive/zip"
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
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/h2non/filetype"
	"github.com/kjk/lzmadec"
)

var FilenameReg *regexp.Regexp

type Archive interface {
	ListDir(string) (files []string, dirs []string, err error)
	GetFileReader(string) (reader io.ReadCloser, err error)
}
type Archive7z struct {
	path    string
	archive *lzmadec.Archive
}

func (a *Archive7z) ListDir(path string) (files []string, dirs []string, err error) {
	files = make([]string, 0)
	dirs = make([]string, 0)
	for _, e := range a.archive.Entries {
		dir, filename := filepath.Split(e.Path)
		if dir == path {
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
func (a *Archive7z) GetFileReader(path string) (reader io.ReadCloser, err error) {
	reader, err = a.archive.GetFileReader(path)
	return
}

type ArchiveZip struct {
	path    string
	archive *zip.ReadCloser
}

func (a *ArchiveZip) ListDir(path string) (files []string, dirs []string, err error) {
	files = make([]string, 0)
	dirs = make([]string, 0)
	for _, file := range a.archive.File {
		dir, filename := filepath.Split(strings.TrimRight(file.Name, "/"))
		if dir == path {
			if file.FileInfo().IsDir() {
				dirs = append(dirs, filename)
			} else {
				files = append(files, filename)
			}
		}
	}
	return
}
func (a *ArchiveZip) GetFileReader(path string) (reader io.ReadCloser, err error) {
	for _, file := range a.archive.File {
		if file.Name == path {
			reader, err = file.Open()
			return
		}
	}
	return
}

type ImageServer struct {
	rootdir  string
	archives map[string]Archive
}

func (s *ImageServer) openArchive(path string) (archive Archive, err error) {
	archive, ok := s.archives[path]
	if ok {
		return
	}
	kind, unknown := filetype.MatchFile(path)
	if unknown != nil {
		err = unknown
		return
	}

	switch kind.Extension {
	case "7z":
		var a *lzmadec.Archive
		a, err = lzmadec.NewArchive(path)
		if err != nil {
			return
		}
		archive = &Archive7z{path, a}
	case "zip":
		var a *zip.ReadCloser
		a, err = zip.OpenReader(path)
		if err != nil {
			return
		}
		archive = &ArchiveZip{path, a}
	}
	s.archives[path] = archive
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

	archive, err := s.openArchive(archivePath)
	if err != nil {
		log.Println(err)
		return
	}
	files, dirs, err = archive.ListDir(pathInArchive)
	if err != nil {
		log.Println(err)
		return
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

type FilenameSort []string

func (s FilenameSort) Less(i, j int) bool {
	gi := FilenameReg.FindStringSubmatch(s[i])
	gj := FilenameReg.FindStringSubmatch(s[j])
	if gi[1] == gj[1] && gi[3] == gj[3] && gi[2] != gj[2] && gi[2] != "" && gj[2] != "" {
		vi, _ := strconv.Atoi(gi[2])
		vj, _ := strconv.Atoi(gj[2])
		return vi < vj
	}
	return s[i] < s[j]
}

func (s FilenameSort) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s FilenameSort) Len() int {
	return len(s)
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
	sort.Sort(FilenameSort(files))
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
	server := &ImageServer{abs_root_dir, make(map[string]Archive)}
	r := mux.NewRouter().SkipClean(true)

	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})
	r.HandleFunc("/list/{path:.*}", server.list)
	r.HandleFunc("/listimg/{path:.*}", server.list)
	r.HandleFunc("/archive_file/{path:.*}", server.archive_file)

	r.PathPrefix("/files/").Handler(http.StripPrefix("/files/", http.FileServer(http.Dir(abs_root_dir))))
	http.Handle("/", r)
	FilenameReg = regexp.MustCompile(`(?m)^(?P<suffix>^.*?)(?P<sn>\d*)(?P<ext>\..*?)?$`)
	log.Fatal(http.ListenAndServe(fmt.Sprintf("%s:%d", *bind_address, *port), r))
}
