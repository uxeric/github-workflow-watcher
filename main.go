package main

import (
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/dustin/go-humanize"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"
)

type Config struct {
	Pat   string `yaml:"pat"`
	Repos []Repo `yaml:"repos"`
}
type Repo struct {
	Name  string `yaml:"name"`
	Owner string `yaml:"owner"`
}
type Workflow struct {
	TotalCount   int           `json:"total_count"`
	WorkflowRuns []WorkflowRun `json:"workflow_runs"`
}
type WorkflowRun struct {
	Id           uint64     `json:"id"`
	UpdatedAt    string     `json:"updated_at"`
	Name         string     `json:"name"`
	HeadBranch   string     `json:"head_branch"`
	DisplayTitle string     `json:"display_title"`
	Status       string     `json:"status"`
	Conclusion   string     `json:"conclusion"`
	Url          string     `json:"url"`
	Actor        Actor      `json:"actor"`
	Repository   Repository `json:"repository"`
}
type Actor struct {
	Id      string `json:"id"`
	Login   string `json:"login"`
	HtmlUrl string `json:"html_url"`
}
type Repository struct {
	Id          string `json:"id"`
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	HtmlUrl     string `json:"html_url"`
	Description string `json:"description"`
}

func main() {
	config := getConfig()
	p := tea.NewProgram(initialModel(config), tea.WithAltScreen())
	if err := p.Start(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}

func getAllWorkflowRuns(repos []Repo, pat string) []WorkflowRun {
	workflowRuns := []WorkflowRun{}
	for _, repo := range repos {
		url := fmt.Sprintf("https://api.github.com/repos/%v/%v/actions/runs", repo.Owner, repo.Name)
		workflow := Workflow{}
		getWorkflow(url, pat, &workflow)
		workflowRuns = append(workflowRuns, workflow.WorkflowRuns...)
	}
	return workflowRuns
}

func getConfig() Config {
	yamlFile, err := ioutil.ReadFile("config.yml")
	if err != nil {
		log.Fatalf("Error reading config.yml: %v", err)
	}
	var config Config
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		log.Fatalf("Error parsing config.yml: %v", err)
	}

	return config
}

func getWorkflow(url string, pat string, data *Workflow) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatalf("Error initializing request: %v", err)
	}
	req.Header = http.Header{
		"Authorization": {"token " + pat},
		"User-Agent":    {"Your mum"},
		"Accept":        {"application/json"},
	}
	res, err := client.Do(req)
	if err != nil {
		log.Fatalf("Error sending request to the Github API %v", err)
	}
	json.NewDecoder(res.Body).Decode(&data)
}

// Application State
type model struct {
	pat          string
	WorkflowRuns []WorkflowRun
	repos        []Repo
	counter      int
	text         string
}

func initialModel(config Config) model {
	allWorkflowRuns := getAllWorkflowRuns(config.Repos, config.Pat)
	return model{
		WorkflowRuns: allWorkflowRuns,
		counter:      1,
		text:         "sup dawg",
		pat:          config.Pat,
		repos:        config.Repos,
	}
}

func (m model) Init() tea.Cmd {
	tea.SetWindowTitle("Github Workflow Watcher")
	return tick()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}
	case tickMsg:
		m.WorkflowRuns = getAllWorkflowRuns(m.repos, m.pat)
		return m, tick()
	}
	return m, nil
}

func (m model) View() string {
	// https://upload.wikimedia.org/wikipedia/commons/1/15/Xterm_256color_chart.svg
	HeaderStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#fdfdfd"))
	EvenRowStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("61"))
	OddRowStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("63"))

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("99"))).
		StyleFunc(func(row, col int) lipgloss.Style {
			switch {
			case row == 0:
				return HeaderStyle
			case row%2 == 0:
				return EvenRowStyle
			default:
				return OddRowStyle
			}
		}).
		Headers("Repo", "Branch", "Description", "User", "Triggered At", "Build Status")

	filteredWorkflowRuns := uniqueWorkflowRuns(m.WorkflowRuns)

	for i, w := range filteredWorkflowRuns {
		dateTime, err := time.Parse(time.RFC3339, w.UpdatedAt)
		if err != nil {
			log.Fatalf("Error covertering UpdatedAt time: %v", err)
		}

		status := w.Conclusion
		if status == "" {
			status = w.Status
		}
		repoName := truncate(w.Repository.Name, 20)
		if sameRepoNameAsLastEntry(i, filteredWorkflowRuns) {
			repoName = ""
		}

		t.Row(
			repoName,
			w.HeadBranch,
			truncate(w.DisplayTitle, 28),
			truncate(w.Actor.Login, 10),
			humanize.Time(dateTime),
			renderStatus(status),
		)
	}

	return t.Render()
}

func renderStatus(status string) string {
	SuccessStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("28"))
	InProgressStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("184"))
	FailedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("160"))

	switch {
	case status == "success":
		return SuccessStyle.Render("Success")
	case status == "in_progress":
		return InProgressStyle.Render("In Progress")
	case status == "queued":
		return InProgressStyle.Render("Queued")
	case status == "failure":
		return FailedStyle.Render("Failed")
	default:
		return status
	}
}

func sameRepoNameAsLastEntry(index int, runs []WorkflowRun) bool {
	if index == 0 {
		return false
	} else if runs[index-1].Repository.Name == runs[index].Repository.Name {
		return true
	} else {
		return false
	}
}

func uniqueWorkflowRuns(workflowRuns []WorkflowRun) []WorkflowRun {
	filteredWorkflowRuns := []WorkflowRun{}
	for _, workflowRun := range workflowRuns {
		if isWorkflowRunUnique(&filteredWorkflowRuns, workflowRun.Repository.Name, workflowRun.HeadBranch) {
			filteredWorkflowRuns = append(filteredWorkflowRuns, workflowRun)
		}
	}
	return filteredWorkflowRuns
}

func isWorkflowRunUnique(selectedRuns *[]WorkflowRun, repoName string, branch string) bool {
	isUnique := true
	for _, run := range *selectedRuns {
		if run.Repository.Name == repoName && run.HeadBranch == branch {
			isUnique = false
			break
		}
	}
	return isUnique
}

func truncate(str string, total int) string {
	if total >= len(str) {
		return str
	} else {
		return fmt.Sprintf("%s...", str[:total])
	}
}

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(7500*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
