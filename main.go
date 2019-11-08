package main

import (
	"bufio"
	"bytes"
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

Example (extract all files specified in the listfile):
	MpqViewer -a -l listfile.txt -mpq_dir /path/to/diablo_ii

Example (extract all files specified in the embedded (listfile) of each MPQ archive):
	MpqViewer -a -mpq_dir /path/to/diablo_ii

Example (extract specific files from d2data.mpq):
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
		listfilePath string
		// Comma-separated list of files to extract.
		rawFilePaths string
	)
	flag.StringVar(&mpqDir, "mpq_dir", ".", "path to Diablo II MPQ directory")
	flag.StringVar(&listfilePath, "l", "listfile.txt", "path to listfile")
	flag.StringVar(&rawFilePaths, "files", "", "comma-separated list of files to extract")
	flag.BoolVar(&all, "a", false, "extract all files")
	flag.Parse()

	// Get MPQ paths.
	mpqPaths := flag.Args()
	if len(mpqPaths) == 0 {
		mpqNames := []string{"d2char.mpq", "d2video.mpq", "d2data.mpq", "d2xmusic.mpq", "d2exp.mpq", "d2xtalk.mpq", "d2music.mpq", "d2xvideo.mpq", "d2sfx.mpq", "d2speech.mpq"} //, "Patch_D2.mpq"}
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
		if len(listfilePath) > 0 {
			fmt.Println("getting file paths from listfile")
			files, err := getFilePathsFromListfile(archives, listfilePath)
			if err != nil {
				log.Fatalf("%+v", err)
			}
			filePaths = files
		} else {
			fmt.Println("getting file paths from embedded (listfile)")
			files, err := getFilePathsFromEmbeddedListfile(archives)
			if err != nil {
				log.Fatalf("%+v", err)
			}
			filePaths = files
		}
	}

	// De-normalize file paths.
	for i, filePath := range filePaths {
		filePaths[i] = denormalize(filePath)
	}

	// Extract files.
	if err := extractAllFiles(archives, filePaths); err != nil {
		log.Fatalf("%+v", err)
	}
}

// getFilePathsFromListfile returns the list of file paths contained within the
// given listfile which are present in any of the MPQ archives.
func getFilePathsFromListfile(archives []mpq.MPQ, listfilePath string) ([]string, error) {
	buf, err := ioutil.ReadFile(listfilePath)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	s := bufio.NewScanner(bytes.NewReader(buf))
	var filePaths []string
	for s.Scan() {
		filePath := s.Text()
		filePath = denormalize(filePath)
		for _, archive := range archives {
			if archive.FileExists(filePath) {
				filePaths = append(filePaths, filePath)
				break
			}
		}
	}
	return filePaths, nil
}

// getFilePathsFromEmbeddedListfile returns the list of file paths contained
// within the embedded (listfile) of each MPQ archive.
func getFilePathsFromEmbeddedListfile(archives []mpq.MPQ) ([]string, error) {
	var filePaths []string
	for _, archive := range archives {
		files, err := archive.GetFileList()
		if err != nil {
			return nil, errors.WithStack(err)
		}
		filePaths = append(filePaths, files...)
	}
	return filePaths, nil
}

// extractAllFiles extracts all files specified by file path from the MPQ
// archives.
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

// extractFile extracts the file from first MPQ archive containing the file
// path.
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

// readFile reads the contents of the given file from the first MPQ archive
// containing the file path.
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

// archiveReadFile reads the contents of the given file from the MPQ archive.
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

// normalize normalizes the file path by replacing backslash characters with
// slash.
func normalize(filePath string) string {
	filePath = strings.ReplaceAll(filePath, `\`, "/")
	return filePath
}

// denormalize de-normalizes the file path by replacing slash characters with
// backslashes and removing any leading slash prefix.
func denormalize(filePath string) string {
	filePath = strings.ReplaceAll(filePath, "/", `\`)
	if strings.HasPrefix(filePath, `\`) {
		filePath = filePath[len(`\`):]
	}
	return filePath
}

var (
	ErrNotFound = errors.New("unable to locate MPQ archive")
	ErrFileRead = errors.New("unable to read file contents")
)
