package main

import (
	"encoding/json"
	"fmt"
	"io"
	whttp "net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v2"

	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/http"
)

type Config struct {
	URL   string `yaml:"url"`
	Token string `yaml:"token"`
	Orgs  []struct {
		Name   string `yaml:"name"`
		Output string `yaml:"output"`
	} `yaml:"orgs"`
}

type Projects struct {
	TotalCount int    `json:"total_count"`
	Items      []Repo `json:"items"`
}

type Repo struct {
	Name     string `json:"name"`
	CloneUrl string `json:"clone_url"`
	SshUrl   string `json:"ssh_url"`
}

func main() {
	// Cargar la configuración desde el archivo config.yaml
	yamlFile, err := os.ReadFile("config.yaml")
	if err != nil {
		fmt.Printf("Error al leer el archivo YAML: %v\n", err)
		return
	}

	// Parsear el archivo YAML al struct "config"
	var config Config
	if err := yaml.Unmarshal(yamlFile, &config); err != nil {
		fmt.Printf("Error al parsear el YAML: %v\n", err)
		return
	}

	// authToken, err := base64.StdEncoding.DecodeString(config.AuthToken)
	// if err != nil {
	// 	fmt.Println("Error al decodificar el token de autenticación:", err)
	// 	return
	// }
	authTokenString := "token " + string(config.Token)

	for _, org := range config.Orgs {
		url := config.URL + "/search/repositories?q=user:" + org.Name
		req, err := whttp.NewRequest("GET", url, nil)
		if err != nil {
			fmt.Println(err)
			return
		}

		req.Header.Add("Authorization", authTokenString)
		// Realizar la solicitud HTTP
		client := &whttp.Client{}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Println(err)
			return
		}
		defer resp.Body.Close()

		// Leer la respuesta
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Println(err)
			return
		}

		var repos Projects
		err = json.Unmarshal(body, &repos)
		if err != nil {
			fmt.Println(err)
			return
		}

		mainsync(org.Output, repos, config.Token)
	}
}

func mainsync(output string, repos Projects, token string) {
	// Obtén la lista de repositorios en el grupo de GitHub
	var wg sync.WaitGroup
	// Recorre los repositorios y clona o actualiza
	for _, repo := range repos.Items {
		repoPath := filepath.Join(output, repo.Name)

		// Si el repositorio no existe localmente, clónalo
		if _, err := os.Stat(repoPath); err == nil {
			fmt.Printf("Pulling: %s\n", repo.Name)
			wg.Add(1)
			// pullRepository(repo, repoPath, token, &wg)
			go pullRepository(repo, repoPath, token, &wg)
		} else {
			// if _, err := os.Stat(repoPath); os.IsNotExist(err) {
			fmt.Printf("Cloning: %s\n", repo.Name)
			wg.Add(1)
			// cloneRepository(repo, repoPath, token, &wg)
			go cloneRepository(repo, repoPath, token, &wg)
		}
	}
	wg.Wait()
}

func cloneRepository(repo Repo, destDir string, accessToken string, wg *sync.WaitGroup) {
	defer wg.Done()

	// Configuración para la clonación con autenticación
	cloneOptions := &git.CloneOptions{
		URL: repo.CloneUrl,
		// Progress: os.Stdout,
		Auth: &http.BasicAuth{Username: "dummy", Password: accessToken}, // El campo Username es necesario pero no se utiliza con tokens de acceso personal
	}

	// Clonar el repositorio
	_, err := git.PlainClone(destDir, false, cloneOptions)

	if err != nil {
		fmt.Printf("Error cloning repository: %s | ERROR: %v\n", repo.Name, err)
	} else {
		fmt.Printf("Repository cloned successfully: %s\n", repo.Name)
	}

}

func pullRepository(repo Repo, destDir string, accessToken string, wg *sync.WaitGroup) {
	// Abrir el repositorio existente
	openRepo, err := git.PlainOpen(destDir)
	if err != nil {
		fmt.Println(err)
	}

	// Configuración para el pull con autenticación
	worktree, err := openRepo.Worktree()
	if err != nil {
		fmt.Println(err)
	}

	pullOptions := &git.PullOptions{
		RemoteName: "origin",
		Auth:       &http.BasicAuth{Username: "dummy", Password: accessToken}, // El campo Username es necesario pero no se utiliza con tokens de acceso personal
	}

	err = worktree.Pull(pullOptions)
	if err == git.NoErrAlreadyUpToDate {
		fmt.Printf("Already up to date: %s\n", repo.Name)
	} else if err == git.ErrUnstagedChanges || err == git.ErrNonFastForwardUpdate {
		// Repasar porque intenta hacer un merge y no quiero
		fmt.Printf("Unstaged changes: %s\n", repo.Name)
		backupDir(destDir)
		cloneRepository(repo, destDir, accessToken, wg)
		return
	} else if err != nil {
		fmt.Printf("Error pulling repository: %s | ERROR: %v\n", repo.Name, err)
	} else {
		fmt.Printf("Repository pulled successfully: %s\n", repo.Name)
	}

	wg.Done()
}

func backupDir(destDir string) {
	now := time.Now()
	newDestDir := fmt.Sprintf("%s_%04d%02d%02d", destDir, now.Year(), now.Month(), now.Day())
	err := os.Rename(destDir, newDestDir)
	if err != nil {
		fmt.Printf("Error on backup: %s | ERROR: %v\n", destDir, err)
	}
}
