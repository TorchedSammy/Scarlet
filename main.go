package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/nstratos/go-myanimelist/mal"
	"github.com/spf13/pflag"
	"github.com/manifoldco/promptui"
	"github.com/oriser/regroup"
)

type jsonConfig struct{
	MalClientID string `json:"malClientID"`
	LibraryDir string `json:"libraryDir"`
	ImportDir string `json:"importDir"`
}
var config jsonConfig

var mangaDirCleanRegex = regexp.MustCompile(`(?mi)\[.+\]`)
var mangaRegex = []*regroup.ReGroup{
	regroup.MustCompile(`(?P<series>.*)c(?P<chapter>\d+\b)`),
	regroup.MustCompile(`(?P<series>.*)v(?P<chapter>\d+\b)`),
}

var exts = []string{
	".cbz",
	".cbr",
}

type Manga struct{
	Name string `regroup:"series"`
	Chapter int `regroup:"chapter"`
	Volume int `regroup:"volume"`
}

type clientIDTransport struct {
	Transport http.RoundTripper
	ClientID  string
}

func (c *clientIDTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if c.Transport == nil {
		c.Transport = http.DefaultTransport
	}
	req.Header.Add("X-MAL-CLIENT-ID", c.ClientID)
	return c.Transport.RoundTrip(req)
}

func initMal() *mal.Client {
	publicInfoClient := &http.Client{
		// Create client ID from https://myanimelist.net/apiconfig. 
		Transport: &clientIDTransport{ClientID: config.MalClientID},
	}

	return mal.NewClient(publicInfoClient)
}

func main() {
	confDir, err := os.UserConfigDir()
	if err != nil {
		fmt.Println("Error getting config dir: ", err)
		os.Exit(1)
	}
	confpath := filepath.Join(confDir, "scarlet", "config.json")
	fmt.Println(confpath)

	confFile, _ := os.ReadFile(confpath)
	if err != nil {
		fmt.Println("Error reading config file: ", err)
		os.Exit(1)
	}
	json.Unmarshal(confFile, &config)

	pflag.StringVarP(&config.LibraryDir, "library", "l", config.LibraryDir, "Where manga is stored")
	skiplns := pflag.BoolP("skiplns", "L", true, "Should Light Novels be hidden in the results")

	pflag.Parse()

	dirs := pflag.Args()
	if len(dirs) == 0 {
		fmt.Println("did not provide directory to import manga.")
	}

	for _, dir := range dirs {
		name := mangaName(dir)
		matches := setupManga(name, dir)
		var lnSkipped int

		fmt.Println("Results:")
		for idx, manga := range matches {
			if *skiplns && manga.MediaType != "manga" {
				lnSkipped++
				continue
			}

			var enTitle string
			if manga.AlternativeTitles.En != "" {
				enTitle = " | " + manga.AlternativeTitles.En
			}
			fmt.Printf("%d. %s%s\n(Volumes: %d, Chapters: %d)\n", idx + 1, manga.Title, enTitle, manga.NumVolumes, manga.NumChapters)
			if idx + 1 == len(matches) {
				fmt.Println("")
			}
		}

		options := []string{
			hil("Options: # selection (default of 1)", "1"),
			hil("Use as-is", "U"),
		}

		for idx, opt := range options {
			fmt.Printf("%s", opt)
			if idx + 1 != len(options) {
				fmt.Print(", ")
			} else {
				fmt.Println("")
			}
		}
		if lnSkipped != 0 {
			fmt.Printf("Skipped %d light novel(s)\n", lnSkipped)
		}

		validate := func(input string) error {
			i, err := strconv.ParseUint(input, 10, 0)
			if err != nil {
				return errors.New("Invalid number")
			}

			if i > uint64(len(matches)) {
				return errors.New("num above amount of matches")
			}
			return nil
		}

		prompt := promptui.Prompt{
			Label: "What to do?",
			Validate: validate,
		}
		result, err := prompt.Run()

		if err != nil {
			fmt.Printf("Prompt failed %v\n", err)
			return
		}

		resInt, _ := strconv.Atoi(result)
		mangaTitle := matches[resInt - 1].Title
		mangaPath := filepath.Join(config.LibraryDir, mangaTitle)
		os.MkdirAll(mangaPath, os.ModePerm)

		files, _ := os.ReadDir(dir)
		for _, f := range files {
			ext := filepath.Ext(f.Name())
			if !slices.Contains(exts, ext) {
				continue
			}

			fmt.Println(f.Name())
			var mangaInfo Manga
			for _, rgx := range mangaRegex {
				err := rgx.MatchToTarget(f.Name(), &mangaInfo)
				if _, ok := err.(*regroup.UnknownGroupError); err != nil && !ok {
					fmt.Println(err)
					continue
				}
				break
			}
			fmt.Println(mangaInfo)

			fnamePieces := []string{mangaTitle}
			if mangaInfo.Volume != 0 {
				fnamePieces = append(fnamePieces, fmt.Sprintf("Vol. %02d", mangaInfo.Volume))
			}
			if mangaInfo.Chapter != 0 {
				fnamePieces = append(fnamePieces, fmt.Sprintf("Ch. %02d", mangaInfo.Chapter))
			}
			filename := strings.Join(fnamePieces, " ") + ext
			fmt.Printf("%s -> %s", filepath.Join(dir, f.Name()), filepath.Join(mangaPath, filename))
			err := os.Link(filepath.Join(dir, f.Name()), filepath.Join(mangaPath, filename))
			fmt.Println(err)
		}
	}
}

func hil(str, char string) string {
	return strings.Replace(str, char, "\u001b[1m\u001b[36m" + char + "\u001b[0m", 1)
}

func mangaName(dir string) string {
	basename := filepath.Base(dir)
	cleaned := mangaDirCleanRegex.ReplaceAllString(basename, "")
	println(cleaned)

	return strings.TrimSpace(cleaned)
}

func setupManga(name, dir string) []mal.Manga {
	c := initMal()
	ctx := context.Background()

	fmt.Printf("searching \"%s\" for directory %s\n", name, dir)

	list, _, err := c.Manga.List(ctx, name,
		mal.Fields{"rank", "num_volumes", "num_chapters", "alternative_titles", "media_type"},
		mal.Limit(5),
	)

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	return list
}
