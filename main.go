package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/OpenDiablo2/OpenDiablo2/mpq"
	"github.com/mewkiz/pkg/pathutil"
	"github.com/pkg/errors"
)

const use = `
Extracts files from MPQ archives.

Usage:
	MpqViewer [OPTION]... [FILE.mpq]...

Example (extract all files from all MPQ archives):
	MpqViewer -a -mpq_dir /path/to/diablo_ii

Example (extract specific file from ):
	MpqViewer -files "/data/global/excel/books.txt,/data/global/excel/charstats.txt" /path/to/d2data.mpq

Flags:
`

func usage() {
	fmt.Fprintln(os.Stderr, use[1:])
	flag.PrintDefaults()
}

func main() {
	// Parse command line arguments.
	var (
		// Extract all files.
		all bool
		// Path to Diablo II MPQ directory.
		mpqDir string
		// Path to listfile.txt
		//listfilePath string
		// Comma-separated list of files to extract.
		rawFilePaths string
	)
	flag.StringVar(&mpqDir, "mpq_dir", ".", "path to Diablo II MPQ directory")
	//flag.StringVar(&listfilePath, "l", "listfile.txt", "path to listfile")
	flag.StringVar(&rawFilePaths, "files", "", "comma-separated list of files to extract")
	flag.BoolVar(&all, "a", false, "extract all files")
	flag.Parse()

	// Get MPQ paths.
	mpqPaths := flag.Args()
	if len(mpqPaths) == 0 {
		mpqNames := []string{"d2char.mpq", "d2video.mpq", "d2data.mpq", "d2xmusic.mpq", "d2exp.mpq", "d2xtalk.mpq", "d2music.mpq", "d2xvideo.mpq", "d2sfx.mpq", "d2speech.mpq"}//, "Patch_D2.mpq"}
		for _, mpqName := range mpqNames {
			mpqPath := filepath.Join(mpqDir, mpqName)
			mpqPaths = append(mpqPaths, mpqPath)
		}
	}

	// Initialize MPQ hash table.
	mpq.InitializeCryptoBuffer()

	// Open MPQ archives.
	var archives []mpq.MPQ
	for _, mpqPath := range mpqPaths {
		archive, err := mpq.Load(mpqPath)
		if err != nil {
			log.Fatalf("%+v", errors.WithStack(err))
		}
		archives = append(archives, archive)
	}

	// Get file paths to extract.
	var filePaths []string
	if len(rawFilePaths) > 0 {
		filePaths = strings.Split(rawFilePaths, ",")
	}
	if len(filePaths) == 0 {
		if !all {
			log.Fatalf("no files to extract specified; specify either FILE or -a")
		}
		files, err := getFilePaths(archives)
		if err != nil {
			log.Fatalf("%+v", err)
		}
		filePaths = files
	}

	// Extract files.
	if err := extractAllFiles(archives, filePaths); err != nil {
		log.Fatalf("%+v", err)
	}
}

func getFilePaths(archives []mpq.MPQ) ([]string, error) {
	var filePaths []string
	for _, archive := range archives {
		fmt.Println("archive:", archive.FileName)
		files, err := archive.GetFileList()
		if err != nil {
			return nil, errors.WithStack(err)
		}
		filePaths = append(filePaths, files...)
	}
	return filePaths, nil
}

func extractAllFiles(archives []mpq.MPQ, filePaths []string) error {
	for _, filePath := range filePaths {
		if err := extractFile(archives, filePath); err != nil {
			switch errors.Cause(err) {
			case ErrNotFound:
				log.Printf("file not found %q\n", filePath)
				continue
			case ErrFileRead:
				log.Printf("file read error %q; %+v\n", filePath, err)
				continue
			}
			return errors.WithStack(err)
		}
	}
	return nil
}

func extractFile(archives []mpq.MPQ, filePath string) error {
	fmt.Printf("extracting %q\n", filePath)
	data, archiveName, err := readFile(archives, filePath)
	if err != nil {
		return errors.WithStack(err)
	}
	archiveDir := pathutil.FileName(archiveName)
	dstPath := normalize(filepath.Join("_dump_", archiveDir, filePath))
	fmt.Printf("creating: %q\n", dstPath)
	dir := filepath.Dir(dstPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.WithStack(err)
	}
	if err := ioutil.WriteFile(dstPath, data, 0644); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func readFile(archives []mpq.MPQ, filePath string) ([]byte, string, error) {
	// de-normalize file name.
	filePath = strings.ToLower(filePath)
	filePath = strings.ReplaceAll(filePath, `/`, "\\")
	if filePath[0] == '\\' {
		filePath = filePath[1:]
	}
	// search for MPQ archive containing file.
	for _, archive := range archives {
		if !archive.FileExists(filePath) {
			continue
		}
		data, err := archiveReadFile(archive, filePath)
		if err != nil {
			return nil, "", errors.WithStack(err)
		}
		return data, archive.FileName, nil
	}
	return nil, "", errors.Wrapf(ErrNotFound, "file not found %q", filePath)
}

func archiveReadFile(archive mpq.MPQ, filePath string) (data []byte, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = errors.Wrap(ErrFileRead, fmt.Sprint(e))
		}
	}()
	data, err = archive.ReadFile(filePath)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return data, err
}

func normalize(filePath string) string {
	filePath = strings.ReplaceAll(filePath, `\`, "/")
	return filePath
}

var (
	ErrNotFound = errors.New("unable to locate MPQ archive")
	ErrFileRead = errors.New("unable to read file contents")
)
