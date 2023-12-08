package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	nurl "net/url"
	"os"
	fp "path/filepath"
	"time"

	"github.com/go-shiori/dom"
	"golang.org/x/net/html"

	"github.com/vissong/go-readability"
)

var httpClient = &http.Client{Timeout: time.Minute}

func main() {
	// Get arguments
	var testName, sourceURL string
	switch len(os.Args) {
	case 2:
		testName = os.Args[1]
	case 3:
		testName = os.Args[1]
		sourceURL = os.Args[2]
	case 0:
		log.Fatalln("need at least one argument")
	default:
		log.Fatalln("allowed max two arguments")
	}

	// Make sure test name is specified
	if testName == "" {
		log.Fatalln("test name must be defined")
	}

	// Make sure URL is valid
	if sourceURL != "" {
		_, err := nurl.ParseRequestURI(sourceURL)
		if err != nil {
			log.Fatalf("URL %s is not valid: %v\n", sourceURL, err)
		}
	}

	// If test name is 'all', generate test case for all existing test directory
	if testName == "all" {
		dirItems, err := ioutil.ReadDir("test-pages")
		if err != nil {
			log.Fatalf("failed to read test dir: %v\n", err)
		}

		for _, item := range dirItems {
			if !item.IsDir() {
				continue
			}

			if !fileExists(fp.Join("test-pages", item.Name(), "source.html")) {
				continue
			}

			err = generateTestcase(item.Name(), "")
			if err != nil {
				log.Fatalf("failed to generate test for %s: %v\n", item.Name(), err)
			}
		}

		return
	}

	err := generateTestcase(testName, sourceURL)
	if err != nil {
		log.Fatalf("failed to generate test for %s: %v\n", testName, err)
	}
}

func generateTestcase(testName, sourceURL string) error {
	log.Println("generating test for", testName)

	// Check if source file for test exists
	// If source file doesn't exist, download it first.
	// If it exist, but URL is defined as well, redownload it
	testDir := fp.Join("test-pages", testName)
	sourcePath := fp.Join(testDir, "source.html")

	if !fileExists(sourcePath) || sourceURL != "" {
		// Download HTML file from URL.
		log.Printf("downloading source for %s from %s\n", testName, sourceURL)
		err := downloadWebPage(sourceURL, sourcePath)
		if err != nil {
			return fmt.Errorf("failed to download source: %v", err)
		}
	}

	// Open and parse source file
	srcFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to open source: %v", err)
	}
	defer srcFile.Close()

	doc, err := dom.Parse(srcFile)
	if err != nil {
		return fmt.Errorf("failed to decode source: %v", err)
	}

	// Extract readable result.
	parsedURL, _ := nurl.ParseRequestURI("http://fakehost/test/page.html")
	article, err := readability.FromDocument(doc, parsedURL)
	if err != nil {
		return fmt.Errorf("failed to parse source: %v", err)
	}

	// Render extracted content to file.
	dstPath := fp.Join(testDir, "expected.html")
	if err = renderNodeToFile(article.Node, dstPath); err != nil {
		return fmt.Errorf("failed to render result: %v", err)
	}

	// Render metadata to file.
	dstPath = fp.Join(testDir, "expected-metadata.json")
	if err = renderMetadataToFile(doc, article, dstPath); err != nil {
		return fmt.Errorf("failed to render metadata: %v", err)
	}

	return nil
}

func fileExists(filePath string) bool {
	info, err := os.Stat(filePath)
	return !os.IsNotExist(err) && !info.IsDir()
}

func downloadWebPage(srcURL string, dstPath string) error {
	// Verify that URL is valid.
	if _, err := nurl.ParseRequestURI(srcURL); err != nil {
		return fmt.Errorf("failed to parse URL: %v", err)
	}

	// Download HTML file from URL.
	resp, err := httpClient.Get(srcURL)
	if err != nil {
		return fmt.Errorf("failed to fetch URL: %v", err)
	}
	defer resp.Body.Close()

	// Save to file
	os.MkdirAll(fp.Dir(dstPath), os.ModePerm)
	dst, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("failed to save file: %v", err)
	}
	defer dst.Close()

	_, err = io.Copy(dst, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to save file: %v", err)
	}

	return nil
}

func renderNodeToFile(element *html.Node, filename string) error {
	dstFile, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create html file: %v", err)
	}
	defer dstFile.Close()

	return html.Render(dstFile, element)
}

func renderMetadataToFile(original *html.Node, article readability.Article, filename string) error {
	dstFile, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create metadata file: %v", err)
	}
	defer dstFile.Close()

	metadata := struct {
		Title      string `json:"title,omitempty"`
		Byline     string `json:"byline,omitempty"`
		Excerpt    string `json:"excerpt,omitempty"`
		Language   string `json:"language,omitempty"`
		SiteName   string `json:"siteName,omitempty"`
		Readerable bool   `json:"readerable"`
	}{
		Title:      article.Title,
		Byline:     article.Byline,
		Excerpt:    article.Excerpt,
		Language:   article.Language,
		SiteName:   article.SiteName,
		Readerable: readability.CheckDocument(original),
	}

	bt, err := json.MarshalIndent(&metadata, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to marshal json: %v", err)
	}

	_, err = dstFile.Write(bt)
	if err != nil {
		return fmt.Errorf("failed to write metadata file: %v", err)
	}

	return dstFile.Sync()
}
