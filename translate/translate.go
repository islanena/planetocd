package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	translate "cloud.google.com/go/translate/apiv3"
	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/aureliengasser/planetocd/articles"
	"github.com/aureliengasser/planetocd/server"
	"github.com/aureliengasser/planetocd/translate/gateway"
	"github.com/gomarkdown/markdown"
	"github.com/urfave/cli/v2"
	translatepb "google.golang.org/genproto/googleapis/cloud/translate/v3"
)

var DEFAULT_GOOGLE_APPLICATION_CREDENTIALS string = os.Getenv("PLANETOCD_GOOGLE_APPLICATION_CREDENTIALS")
var DEFAULT_DEEPL_TOKEN_PATH = os.Getenv("PLANETOCD_DEEPL_TOKEN_PATH")
var DEFAULT_INPUT_MD_FILE = "./workdir/in.md"
var DEFAULT_INPUT_HTML_FILE = "./workdir/in.html"
var DEFAULT_OUTPUT_DIR = "./articles/articles/"
var DEFAULT_PAGE_NUMBER = 1

func main() {

	var articleId int
	var articleOriginalTitle string
	var articleOriginalURL string
	var articleOriginalAuthor string
	var articlePageNumber int
	var articleInputFileMD string
	var articleInputFileHTML string
	var articleOutPath string

	var fileToken string
	var fileTargetLanguage string
	var fileInputExtension string

	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") == "" {
		os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", DEFAULT_GOOGLE_APPLICATION_CREDENTIALS)
	}

	app := &cli.App{
		Commands: []*cli.Command{
			{
				Name:  "article",
				Usage: "Translate an article",
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:        "id",
						Usage:       "Output article ID",
						Required:    true,
						Destination: &articleId,
					},
					&cli.StringFlag{
						Name:        "title",
						Usage:       "Original article title",
						Required:    true,
						Destination: &articleOriginalTitle,
					},
					&cli.StringFlag{
						Name:        "url",
						Usage:       "Original article url",
						Required:    true,
						Destination: &articleOriginalURL,
					},
					&cli.StringFlag{
						Name:        "author",
						Usage:       "Original article Author",
						Destination: &articleOriginalAuthor,
					},
					&cli.IntFlag{
						Name:        "page",
						Usage:       "Page number",
						Value:       DEFAULT_PAGE_NUMBER,
						Destination: &articlePageNumber,
					},
					&cli.StringFlag{
						Name:        "input-md",
						Usage:       "Input Markdown file path",
						Value:       DEFAULT_INPUT_MD_FILE,
						Destination: &articleInputFileMD,
					},
					&cli.StringFlag{
						Name:        "input-html",
						Usage:       "Input Markdown HTML file path",
						Value:       DEFAULT_INPUT_HTML_FILE,
						Destination: &articleInputFileHTML,
					},
					&cli.StringFlag{
						Name:        "output-path",
						Usage:       "Output article directory",
						Value:       DEFAULT_OUTPUT_DIR,
						Destination: &articleOutPath,
					},
				},
				Action: func(c *cli.Context) error {
					CreateTranslatedArticle(
						articleId,
						articleOriginalTitle,
						articleOriginalURL,
						articleOriginalAuthor,
						articlePageNumber,
						articleInputFileMD,
						articleInputFileHTML,
						articleOutPath)
					return nil
				},
			},
			{
				Name:  "file",
				Usage: "Translate a file",

				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:        "token",
						Usage:       "Access token",
						Destination: &fileToken,
						Required:    false,
					},
					&cli.StringFlag{
						Name:        "lang",
						Usage:       "Target language",
						Destination: &fileTargetLanguage,
						Required:    true,
					},
					&cli.StringFlag{
						Name:        "ext",
						Usage:       "Input string extension (corresponding to MIME type)",
						Destination: &fileInputExtension,
						Required:    false,
					},
				},
				Action: func(c *cli.Context) error {
					TranslateFile(
						c.Args().Get(0),
						fileInputExtension,
						fileToken,
						fileTargetLanguage)
					return nil
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func TranslateFile(inputFile string, inputExtension string, token string, targetLanguage string) {

	if token == "" {
		file, err := os.Open(DEFAULT_DEEPL_TOKEN_PATH)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()
		tokenB, err := io.ReadAll(file)
		if err != nil {
			log.Fatal(err)
		}
		token = string(tokenB)
	}

	inputText := ""

	if inputFile == "-" {
		reader := bufio.NewReader(os.Stdin)
		inputB, _ := io.ReadAll(reader)
		inputText = string(inputB)
	} else {
		file, err := os.Open(inputFile)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()
		inputB, err := io.ReadAll(file)
		if err != nil {
			log.Fatal(err)
		}
		inputText = string(inputB)
		if inputExtension == "" {
			inputExtension = filepath.Ext(inputFile)
		}
	}

	text, err := gateway.Translate(
		inputText,
		inputExtension,
		strings.ToUpper(targetLanguage),
		token,
		gateway.FORMALITY_MORE,
	)

	if err != nil {
		log.Fatalln(err)
	}

	fmt.Print(text)
}

// CreateTranslatedArticle ....
func CreateTranslatedArticle(
	id int,
	originalTitle string,
	originalURL string,
	originalAuthor string,
	pageNumber int,
	inputFileMD string,
	inputFileHTML string,
	outPath string) {

	idStr := fmt.Sprintf("%04d", id)
	inputMD, err := ioutil.ReadFile(inputFileMD)

	if err != nil {
		log.Fatal(err)
	}

	html := markdown.ToHTML(inputMD, nil, nil)
	slug := server.Slugify(originalTitle)

	metadata := articles.ArticleMetadata{
		OriginalURL:    originalURL,
		OriginalTitle:  originalTitle,
		OriginalAuthor: originalAuthor,
		Languages:      make(map[string]articles.ArticleLanguageMetadata),
		PublishedDate:  time.Now(),
		Tags:           []string{},
	}

	var existingMetadata articles.ArticleMetadata
	metadataFilePath := path.Join(outPath, idStr+"__"+slug+".json")
	metadataFile, err := ioutil.ReadFile(metadataFilePath)
	if err == nil {
		err := json.Unmarshal(metadataFile, &existingMetadata)
		if err != nil {
			log.Fatal(err)
		}
	}

	for _, lang := range server.SupportedLanguages {
		fileName, err := translateAndWrite(outPath, lang, string(html), idStr, pageNumber)
		if err != nil {
			log.Fatal(err)
		}
		translatedTitle, err := translateText(os.Stdout, "planetocd", "en", lang, originalTitle, "text/plain", "default")
		if err != nil {
			log.Fatal(err)
		}

		var pages []string

		if pageNumber == 1 {
			pages = []string{fileName}
		} else {
			if _, ok := existingMetadata.Languages[lang]; !ok {
				log.Fatal("Couldn't find existing metadata for language: " + lang)
			}
			if len(existingMetadata.Languages[lang].Pages) != pageNumber-1 {
				log.Fatalf("Invalid existing metadata for language: %v. Existing metadata has %v pages.", lang, existingMetadata.Languages[lang].Pages)
			}
			pages = append(existingMetadata.Languages[lang].Pages, fileName)
		}

		metadata.Languages[lang] = articles.ArticleLanguageMetadata{
			Title: strings.Trim(translatedTitle, "\n"),
			Pages: pages,
		}
	}

	metadataJSON, err := json.MarshalIndent(&metadata, "", "    ")
	if err != nil {
		log.Fatal(err)
	}
	ioutil.WriteFile(metadataFilePath, metadataJSON, 0644)

	copyFile(inputFileMD, path.Join(outPath, idStr+"__original.md"))
	copyFile(inputFileHTML, path.Join(outPath, idStr+"__original.html"))
}

func translateAndWrite(outPath string, lang string, html string, id string, pageNumber int) (string, error) {
	translatedHTML, err := translateText(os.Stdout, "planetocd", "en", lang, html, "text/html", "default")
	if err != nil {
		log.Fatal(err)
	}

	converter := md.NewConverter("", true, nil)
	markdown, err := converter.ConvertString(translatedHTML)
	if err != nil {
		log.Fatal(err)
	}
	fileName := id + "_" + lang + "_0" + strconv.Itoa(pageNumber) + ".md"
	ioutil.WriteFile(path.Join(outPath, fileName), []byte(markdown), 0644)
	return fileName, nil
}

func copyFile(src string, dest string) {
	input, err := ioutil.ReadFile(src)
	if err != nil {
		log.Fatal(err)
		return
	}

	err = ioutil.WriteFile(dest, input, 0644)
	if err != nil {
		fmt.Println("Error creating", dest)
		log.Fatal(err)
		return
	}
}

func translateText(w io.Writer, projectID string, sourceLang string, targetLang string, text string, mimeType string, modelType string) (string, error) {
	ctx := context.Background()
	client, err := translate.NewTranslationClient(ctx)
	if err != nil {
		return "", fmt.Errorf("NewTranslationClient: %v", err)
	}
	defer client.Close()

	model := ""
	if modelType != "default" {
		model = "projects/planetocd/locations/global/models/general/" + modelType
	}

	req := &translatepb.TranslateTextRequest{
		Parent:             fmt.Sprintf("projects/%s/locations/global", projectID),
		SourceLanguageCode: sourceLang,
		TargetLanguageCode: targetLang,
		Model:              model,    // nmt or base
		MimeType:           mimeType, // Mime types: "text/plain", "text/html"
		Contents:           []string{text},
	}

	resp, err := client.TranslateText(ctx, req)
	if err != nil {
		return "", fmt.Errorf("TranslateText: %v", err)
	}

	res := ""
	for _, translation := range resp.GetTranslations() {
		res = res + translation.GetTranslatedText() + "\n"
	}

	return res, nil
}
