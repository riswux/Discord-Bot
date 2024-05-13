package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/sheets/v4"
)

func initSheetsService() (*sheets.Service, error) {
	b, err := ioutil.ReadFile("credentials.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	config, err := google.ConfigFromJSON(b, sheets.SpreadsheetsScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}

	client := getClient(config)
	srv, err := sheets.New(client)
	if err != nil {
		log.Fatalf("Unable to retrieve Sheets client: %v", err)
	}
	return srv, nil
}

// getClient retrieves an HTTP client using OAuth configurations.
func getClient(config *oauth2.Config) *http.Client {
	tok, err := tokenFromFile(sheetsTokenFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		if tok == nil { // Verify token was actually retrieved
			return nil
		}
		saveToken(sheetsTokenFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// getTokenFromWeb handles the web-based OAuth flow to retrieve a new token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Println("Go to the following link in your browser then type the authorization code: ")
	fmt.Println(authURL)

	var authCode string
	fmt.Print("Enter the authorization code here: ")
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Printf("Unable to read authorization code: %v", err)
		return nil
	}

	tok, err := config.Exchange(context.Background(), authCode)
	if err != nil {
		log.Printf("Unable to retrieve token from web: %v", err)
		return nil
	}
	return tok
}

// tokenFromFile reads an OAuth token from a file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("unable to open token file: %v", err)
	}
	defer f.Close()

	tok := &oauth2.Token{}
	if err = json.NewDecoder(f).Decode(tok); err != nil {
		return nil, fmt.Errorf("unable to decode token: %v", err)
	}
	return tok, nil
}

// saveToken saves an OAuth token to a file.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()

	if err := json.NewEncoder(f).Encode(token); err != nil {
		log.Fatalf("Unable to encode token to file: %v", err)
	}
}
