package mmc

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"path/filepath"

	"github.com/xuri/excelize/v2"
)

const (
	// accountsFilePattern is the pattern to match the accounts file
	accountsFilePattern = "?ccounts*.xlsx"
	sheetName           = "Sheet1"

	// headers for the accounts file
	nameHeader       = "Name"
	emailHeader      = "Email"
	githubUserHeader = "GitHub User"
)

type Accounts []student

func (a *Accounts) GetRepoName(user string) (string, error) {
	for _, acc := range *a {
		if acc.GithubUser == user {
			return acc.RepoName(), nil
		}
	}
	return "", fmt.Errorf("GitHub user %s not found", user)
}

var (
	ErrAccountsNotFound = errors.New("no classroom found: run `gh mmc init` in a classroom folder or in a folder containing an accounts file [Aa]ccounts*.xlsx")
)

// FindAccountsFolder searches upwards from the current directory to find a folder containing an accounts file
// Returns the absolute path to the folder, or an error if not found
func FindAccountsFolder() (string, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %v", err)
	}

	for {
		entries, err := os.ReadDir(currentDir)
		if err == nil {
			for _, e := range entries {
				if matched, _ := filepath.Match(accountsFilePattern, e.Name()); matched && !e.IsDir() {
					return currentDir, nil
				}
			}
		}

		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			return "", ErrAccountsNotFound
		}

		currentDir = parentDir
	}
}

// check if an account file is available in the current folder and return the name of it
func getAccountFile() (string, error) {
	files, err := filepath.Glob(accountsFilePattern)
	if err != nil {
		return "", err
	}
	if len(files) != 1 {
		return "", fmt.Errorf("no accounts file found")
	}
	return files[0], nil
}

// ReadAccounts reads the accounts from the accounts file
func ReadAccounts() ([]student, error) {
	// find the accounts file
	accountFile, err := getAccountFile()
	if err != nil {
		return nil, fmt.Errorf("failed to find accounts file: %v", err)
	}

	// open the accounts file
	file, err := excelize.OpenFile(accountFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open accounts file: %v", err)
	}
	defer file.Close() //nolint:errcheck

	// get the rows from the sheet
	rows, err := file.GetRows(sheetName)
	if err != nil {
		return nil, fmt.Errorf("failed to get rows from sheet: %v", err)
	}

	// create a map to hold the accounts
	accounts := make([]map[string]string, 0)

	// iterate over the rows, starting from the second row
	for _, row := range rows[1:] {
		// create a map to hold the account data
		account := make(map[string]string)
		for i, cell := range row {
			account[rows[0][i]] = strings.TrimSpace(cell)
		}
		accounts = append(accounts, account)
	}

	// check if there are any accounts
	if len(accounts) == 0 {
		return nil, fmt.Errorf("no students found")
	}

	// check if therea are the right headers
	if _, ok := accounts[0][nameHeader]; !ok {
		return nil, fmt.Errorf("no Name column found")
	}
	if _, ok := accounts[0][emailHeader]; !ok {
		return nil, fmt.Errorf("no Email column found")
	}
	if _, ok := accounts[0][githubUserHeader]; !ok {
		return nil, fmt.Errorf("no GitHub User column found")
	}

	// create a slice to hold the account structs
	accountList := make([]student, 0)
	for _, a := range accounts {
		accountList = append(accountList, student{
			Name:       a[nameHeader],
			Email:      a[emailHeader],
			GithubUser: a[githubUserHeader],
		})
	}

	return accountList, nil
}
