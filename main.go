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
	"strconv"
	"strings"

	"github.com/nstratos/go-myanimelist/mal"
	"github.com/spf13/pflag"
	"github.com/manifoldco/promptui"
)

type jsonConfig struct{
	MalClientID string `json:"malClientID"`
}
var config jsonConfig

var mangaDirCleanRegex = regexp.MustCompile(`(?mi)\[.+\]`)

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

	pflag.Parse()

	dirs := pflag.Args()
	if len(dirs) == 0 {
		fmt.Println("did not provide directory to import manga.")
	}

	for _, dir := range dirs {
		name := mangaName(dir)
		matches := setupManga(name, dir)

		fmt.Println("Results:")
		for idx, manga := range matches {
			fmt.Printf("%d. %s (Volumes: %d, Chapters: %d)\n", idx + 1, manga.Title, manga.NumVolumes, manga.NumChapters)
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

		validate := func(input string) error {
			_, err := strconv.ParseFloat(input, 64)
			if err != nil {
				return errors.New("Invalid number")
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
		fmt.Println(result)
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
		mal.Fields{"rank", "num_volumes", "num_chapters"},
		mal.Limit(5),
	)

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	return list
}
