package codespaces

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/majikmate/gh-mmc/pkg/ghapi"
	"github.com/majikmate/gh-mmc/pkg/mmc"
	"github.com/spf13/cobra"
)

func NewCmdCodespaces(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "codespaces",
		Short: "Manage codespaces for organizations",
		Long: heredoc.Doc(`
		
			Manage codespaces owned by organizations, including listing and removing them.

			When run inside an assignment folder, commands will only show codespaces for 
			repositories belonging to that assignment. Otherwise, shows all codespaces 
			for the organization.

			The organization is looked up from the classroom metadata if it exists, 
			otherwise you will be prompted to select an organization from your available 
			organizations.`),
	}

	cmd.AddCommand(NewCmdCodespacesList(f))
	cmd.AddCommand(NewCmdCodespacesRm(f))

	return cmd
}

func NewCmdCodespacesList(f *cmdutil.Factory) *cobra.Command {
	var orgName string
	var verbose bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all codespaces owned by a specific organization",
		Long: heredoc.Doc(`
		
			Lists all codespaces owned by a specific organization, including their active 
			state and machine information.

			When run inside an assignment folder, only shows codespaces for repositories 
			belonging to that assignment. When run inside a classroom folder (but not an 
			assignment folder), shows codespaces for all repositories belonging to that 
			classroom. Otherwise, shows all codespaces for the organization.

			The organization is looked up from the classroom metadata if it exists, 
			otherwise you will be prompted to select an organization from your available 
			organizations.

			For each codespace, the command shows detailed information including machine 
			specifications, prebuild status, and last usage time.`),
		Example: `$ gh mmc codespaces list
$ gh mmc codespaces list --org my-org`,
		Run: func(cmd *cobra.Command, args []string) {
			client, err := api.DefaultRESTClient()
			if err != nil {
				mmc.Fatal(fmt.Errorf("failed to create gh client: %v", err))
			}

			// Try to get organization from classroom metadata
			if orgName == "" {
				c, err := mmc.LoadClassroom()
				if err != nil {
					if errors.Is(err, mmc.ErrClassroomNotFound) {
						// Prompt for organization selection
						org, err := ghapi.PromptForOrganization(client)
						if err != nil {
							mmc.Fatal(fmt.Errorf("failed to select organization: %v", err))
						}
						orgName = org.Login
					} else {
						mmc.Fatal(err)
					}
				} else {
					orgName = c.Organization.Login
				}
			}

			fmt.Printf("Fetching codespaces for organization: %s", orgName)

			// Get codespaces for the organization
			codespaces, err := ghapi.GetCodespacesForOrg(client, orgName)
			if err != nil {
				mmc.Fatal(fmt.Errorf("failed to get codespaces: %v", err))
			}

			// Check if we're in an assignment folder and filter accordingly
			a, err := mmc.LoadAssignment()
			if err == nil {
				// We're in an assignment folder, filter codespaces by assignment repositories
				fmt.Printf(" (filtered by assignment: %s)\n", a.Name)

				// Get accepted assignments for this assignment
				acceptedAssignmentList, err := ghapi.ListAllAcceptedAssignments(client, a.Id, 15)
				if err != nil {
					mmc.Fatal(fmt.Errorf("failed to get accepted assignments: %v", err))
				}

				// Create a map of repository full names for quick lookup
				assignmentRepos := make(map[string]bool)
				for _, acceptedAssignment := range acceptedAssignmentList.AcceptedAssignments {
					assignmentRepos[acceptedAssignment.Repository.FullName] = true
				}

				// Filter codespaces to only include those from assignment repositories
				var filteredCodespaces []ghapi.GitHubCodespace
				for _, cs := range codespaces {
					if assignmentRepos[cs.Repository.FullName] {
						filteredCodespaces = append(filteredCodespaces, cs)
					}
				}
				codespaces = filteredCodespaces
			} else if !errors.Is(err, mmc.ErrAssignmentNotFound) {
				mmc.Fatal(fmt.Errorf("failed to check assignment context: %v", err))
			} else {
				// We're not in an assignment folder, but check if we're in a classroom folder
				// If so, filter codespaces by all classroom assignments
				c, err := mmc.LoadClassroom()
				if err == nil {
					fmt.Printf(" (filtered by classroom: %s)\n", c.Classroom.Name)

					// Get all assignments for this classroom
					allAssignments, err := ghapi.ListAllAssignments(client, c.Classroom.Id)
					if err != nil {
						mmc.Fatal(fmt.Errorf("failed to get classroom assignments: %v", err))
					}

					// Collect all repository full names from all assignments
					classroomRepos := make(map[string]bool)

					// For each assignment, get all accepted assignments and their repositories
					for _, assignment := range allAssignments {
						acceptedAssignmentList, err := ghapi.ListAllAcceptedAssignments(client, assignment.Id, 15)
						if err != nil {
							// Log error but continue with other assignments
							fmt.Printf("Warning: failed to get accepted assignments for assignment %s: %v\n", assignment.Title, err)
							continue
						}

						for _, acceptedAssignment := range acceptedAssignmentList.AcceptedAssignments {
							classroomRepos[acceptedAssignment.Repository.FullName] = true
						}

						// Also include the starter code repository if it exists
						if assignment.StarterCodeRepository.Id != 0 {
							classroomRepos[assignment.StarterCodeRepository.FullName] = true
						}
					}

					// Filter codespaces to only include those from classroom repositories
					var filteredCodespaces []ghapi.GitHubCodespace
					for _, cs := range codespaces {
						if classroomRepos[cs.Repository.FullName] {
							filteredCodespaces = append(filteredCodespaces, cs)
						}
					}
					codespaces = filteredCodespaces
				} else {
					fmt.Println()
				}
			}
			fmt.Println()

			if len(codespaces) == 0 {
				fmt.Printf("No codespaces found for organization %s\n", orgName)
				return
			}

			// Print header with fixed-width formatting to handle emoji alignment
			fmt.Printf("%-25s %-6s %-35s %-25s %-42s %-8s %-5s %s\n",
				"NAME", "GIT", "REPOSITORY", "USER", "MACHINE", "IDLE", "PRE", "LAST USED")

			// Load classroom context once for student name lookups
			classroom, classroomErr := mmc.LoadClassroom()

			// Create a slice to hold codespace data with student names for sorting
			type codespaceWithStudent struct {
				codespace   ghapi.GitHubCodespace
				studentName string
			}

			var codespacesList []codespaceWithStudent

			// Populate the list with student names
			for _, cs := range codespaces {
				var studentName string
				if classroomErr == nil {
					if name, err := classroom.GetRepoName(cs.Owner.Login); err == nil {
						studentName = name
					}
				}
				codespacesList = append(codespacesList, codespaceWithStudent{
					codespace:   cs,
					studentName: studentName,
				})
			}

			// Sort by student name (empty names go to the end)
			sort.Slice(codespacesList, func(i, j int) bool {
				// If one student name is empty and the other isn't, put empty ones at the end
				if codespacesList[i].studentName == "" && codespacesList[j].studentName != "" {
					return false
				}
				if codespacesList[i].studentName != "" && codespacesList[j].studentName == "" {
					return true
				}
				// If both student names are empty, sort by owner (GitHub username)
				if codespacesList[i].studentName == "" && codespacesList[j].studentName == "" {
					return codespacesList[i].codespace.Owner.Login < codespacesList[j].codespace.Owner.Login
				}
				// Both are non-empty, sort alphabetically by student name
				return codespacesList[i].studentName < codespacesList[j].studentName
			})

			for _, item := range codespacesList {
				cs := item.codespace
				studentName := item.studentName

				// Use student name if available, otherwise use GitHub username
				var displayUser string
				if studentName != "" {
					displayUser = studentName
				} else {
					displayUser = cs.Owner.Login
				}

				// Truncate user name if too long
				if len(displayUser) > 24 {
					displayUser = displayUser[:21] + "..."
				}

				// Format machine information with consistent padding
				memoryGB := cs.Machine.MemoryInBytes / (1024 * 1024 * 1024)
				storageGB := cs.Machine.StorageInBytes / (1024 * 1024 * 1024)
				machineInfo := fmt.Sprintf("%2d cores, %2d GB RAM, %2d GB storage (%s)",
					cs.Machine.CPUs, memoryGB, storageGB, cs.Machine.OperatingSystem)

				// Handle nullable PrebuildAvailability
				var availability string
				if cs.Machine.PrebuildAvailability != nil {
					availability = *cs.Machine.PrebuildAvailability
				}
				prebuildInfo := formatPrebuild(cs.Prebuild, availability)

				lastUsed := "Never"
				if cs.LastUsedAt != nil && *cs.LastUsedAt != "" {
					if t, err := time.Parse(time.RFC3339, *cs.LastUsedAt); err == nil {
						lastUsed = t.Format("Mon 2006-01-02 15:04")
					}
				}

				// Truncate long names and repositories for better formatting
				displayName := cs.DisplayName
				if len(displayName) > 24 {
					displayName = displayName[:21] + "..."
				}

				// Strip organization prefix from repository name since all repos belong to the same org
				repoName := cs.Repository.FullName
				if orgPrefix := orgName + "/"; strings.HasPrefix(repoName, orgPrefix) {
					repoName = repoName[len(orgPrefix):]
				}
				if len(repoName) > 34 {
					repoName = repoName[:31] + "..."
				}

				// Format idle timeout
				idleTimeout := fmt.Sprintf("%dm", cs.IdleTimeoutMinutes)

				// Format git status
				gitStatus := formatGitStatus(cs.GitStatus)

				// Add color coding based on state
				var colorStart, colorEnd string
				switch cs.State {
				case "Available":
					colorStart = "\033[32m" // Green
					colorEnd = "\033[0m"    // Reset
				case "Shutdown":
					colorStart = "" // Default terminal color
					colorEnd = ""
				default:
					colorStart = "\033[33m" // Yellow
					colorEnd = "\033[0m"    // Reset
				}

				fmt.Printf("%s%-25s %-6s %-35s %-25s %-42s %-8s %-5s %s%s\n",
					colorStart,
					displayName,
					gitStatus,
					repoName,
					displayUser,
					machineInfo,
					idleTimeout,
					prebuildInfo,
					lastUsed,
					colorEnd,
				)
			}

			fmt.Printf("\nTotal codespaces: %d\n", len(codespaces))
		},
	}

	cmd.Flags().StringVarP(&orgName, "org", "o", "", "Organization name (if not provided, will be detected from classroom metadata or prompted)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose error output")

	return cmd
}

func NewCmdCodespacesRm(f *cmdutil.Factory) *cobra.Command {
	var orgName string
	var verbose bool
	var all bool

	cmd := &cobra.Command{
		Use:   "rm",
		Short: "Remove selected codespaces for an organization",
		Long: heredoc.Doc(`
		
			Interactively select and remove codespaces for a specific organization.

			When run inside an assignment folder, only shows codespaces for repositories 
			belonging to that assignment. When run inside a classroom folder (but not an 
			assignment folder), shows codespaces for all repositories belonging to that 
			classroom. Otherwise, shows all codespaces for the organization.

			This command will show you all available codespaces and allow you to 
			select which ones to delete. You can select multiple codespaces at once.

			Use the --all flag to automatically delete all non-running codespaces 
			without interactive selection. For safety, --all only deletes codespaces 
			with clean git status (no uncommitted or unpushed changes).

			The organization is looked up from the classroom metadata if it exists, 
			otherwise you will be prompted to select an organization from your available 
			organizations.`),
		Example: `$ gh mmc codespaces rm
$ gh mmc codespaces rm --org my-org
$ gh mmc codespaces rm --all
$ gh mmc codespaces rm --org my-org --all`,
		Run: func(cmd *cobra.Command, args []string) {
			client, err := api.DefaultRESTClient()
			if err != nil {
				mmc.Fatal(fmt.Errorf("failed to create gh client: %v", err))
			}

			// Try to get organization from classroom metadata
			if orgName == "" {
				c, err := mmc.LoadClassroom()
				if err != nil {
					if errors.Is(err, mmc.ErrClassroomNotFound) {
						// Prompt for organization selection
						org, err := ghapi.PromptForOrganization(client)
						if err != nil {
							mmc.Fatal(fmt.Errorf("failed to select organization: %v", err))
						}
						orgName = org.Login
					} else {
						mmc.Fatal(err)
					}
				} else {
					orgName = c.Organization.Login
				}
			}

			fmt.Printf("Fetching codespaces for organization: %s", orgName)

			// Get codespaces for the organization
			codespaces, err := ghapi.GetCodespacesForOrg(client, orgName)
			if err != nil {
				mmc.Fatal(fmt.Errorf("failed to get codespaces: %v", err))
			}

			// Check if we're in an assignment folder and filter accordingly
			a, err := mmc.LoadAssignment()
			if err == nil {
				// We're in an assignment folder, filter codespaces by assignment repositories
				fmt.Printf(" (filtered by assignment: %s)\n", a.Name)

				// Get accepted assignments for this assignment
				acceptedAssignmentList, err := ghapi.ListAllAcceptedAssignments(client, a.Id, 15)
				if err != nil {
					mmc.Fatal(fmt.Errorf("failed to get accepted assignments: %v", err))
				}

				// Create a map of repository full names for quick lookup
				assignmentRepos := make(map[string]bool)
				for _, acceptedAssignment := range acceptedAssignmentList.AcceptedAssignments {
					assignmentRepos[acceptedAssignment.Repository.FullName] = true
				}

				// Filter codespaces to only include those from assignment repositories
				var filteredCodespaces []ghapi.GitHubCodespace
				for _, cs := range codespaces {
					if assignmentRepos[cs.Repository.FullName] {
						filteredCodespaces = append(filteredCodespaces, cs)
					}
				}
				codespaces = filteredCodespaces
			} else if !errors.Is(err, mmc.ErrAssignmentNotFound) {
				mmc.Fatal(fmt.Errorf("failed to check assignment context: %v", err))
			} else {
				// We're not in an assignment folder, but check if we're in a classroom folder
				// If so, filter codespaces by all classroom assignments
				c, err := mmc.LoadClassroom()
				if err == nil {
					fmt.Printf(" (filtered by classroom: %s)\n", c.Classroom.Name)

					// Get all assignments for this classroom
					allAssignments, err := ghapi.ListAllAssignments(client, c.Classroom.Id)
					if err != nil {
						mmc.Fatal(fmt.Errorf("failed to get classroom assignments: %v", err))
					}

					// Collect all repository full names from all assignments
					classroomRepos := make(map[string]bool)

					// For each assignment, get all accepted assignments and their repositories
					for _, assignment := range allAssignments {
						acceptedAssignmentList, err := ghapi.ListAllAcceptedAssignments(client, assignment.Id, 15)
						if err != nil {
							// Log error but continue with other assignments
							fmt.Printf("Warning: failed to get accepted assignments for assignment %s: %v\n", assignment.Title, err)
							continue
						}

						for _, acceptedAssignment := range acceptedAssignmentList.AcceptedAssignments {
							classroomRepos[acceptedAssignment.Repository.FullName] = true
						}

						// Also include the starter code repository if it exists
						if assignment.StarterCodeRepository.Id != 0 {
							classroomRepos[assignment.StarterCodeRepository.FullName] = true
						}
					}

					// Filter codespaces to only include those from classroom repositories
					var filteredCodespaces []ghapi.GitHubCodespace
					for _, cs := range codespaces {
						if classroomRepos[cs.Repository.FullName] {
							filteredCodespaces = append(filteredCodespaces, cs)
						}
					}
					codespaces = filteredCodespaces
				} else {
					fmt.Println()
				}
			}
			fmt.Println()

			if len(codespaces) == 0 {
				fmt.Printf("No codespaces found for organization %s\n", orgName)
				return
			}

			// Sort codespaces by student name for consistent ordering
			classroom, classroomErr := mmc.LoadClassroom()

			// Create a slice to hold codespace data with student names for sorting
			type codespaceWithStudent struct {
				codespace   ghapi.GitHubCodespace
				studentName string
			}

			var codespacesList []codespaceWithStudent

			// Populate the list with student names
			for _, cs := range codespaces {
				var studentName string
				if classroomErr == nil {
					if name, err := classroom.GetRepoName(cs.Owner.Login); err == nil {
						studentName = name
					}
				}
				codespacesList = append(codespacesList, codespaceWithStudent{
					codespace:   cs,
					studentName: studentName,
				})
			}

			// Sort by student name (empty names go to the end)
			sort.Slice(codespacesList, func(i, j int) bool {
				// If one student name is empty and the other isn't, put empty ones at the end
				if codespacesList[i].studentName == "" && codespacesList[j].studentName != "" {
					return false
				}
				if codespacesList[i].studentName != "" && codespacesList[j].studentName == "" {
					return true
				}
				// If both student names are empty, sort by owner (GitHub username)
				if codespacesList[i].studentName == "" && codespacesList[j].studentName == "" {
					return codespacesList[i].codespace.Owner.Login < codespacesList[j].codespace.Owner.Login
				}
				// Both are non-empty, sort alphabetically by student name
				return codespacesList[i].studentName < codespacesList[j].studentName
			})

			// Extract sorted codespaces back to the original slice
			codespaces = make([]ghapi.GitHubCodespace, len(codespacesList))
			for i, item := range codespacesList {
				codespaces[i] = item.codespace
			}

			var selectedCodespaces []ghapi.GitHubCodespace

			if all {
				// Filter non-running codespaces without uncommitted/unpushed changes when using --all flag
				var filteredCount int
				for _, cs := range codespaces {
					if cs.State != "Available" && !cs.GitStatus.HasUncommittedChanges && !cs.GitStatus.HasUnpushedChanges {
						selectedCodespaces = append(selectedCodespaces, cs)
					} else if cs.State != "Available" {
						filteredCount++ // Count filtered out non-running codespaces
					}
				}

				if len(selectedCodespaces) == 0 {
					if filteredCount > 0 {
						fmt.Printf("No clean non-running codespaces found to delete.\n")
						fmt.Printf("Found %d non-running codespace(s) with uncommitted or unpushed changes (skipped for safety).\n", filteredCount)
					} else {
						fmt.Println("No non-running codespaces found to delete.")
					}
					return
				}

				fmt.Printf("Found %d clean non-running codespace(s) to delete with --all flag:\n", len(selectedCodespaces))
				if filteredCount > 0 {
					fmt.Printf("(Skipped %d non-running codespace(s) with uncommitted or unpushed changes)\n", filteredCount)
				}
				fmt.Println()

				// Display in table format similar to interactive selection
				displayCodespacesTable(selectedCodespaces, orgName)
			} else {
				// Prompt user to select codespaces to delete
				var err error

				// Create getUserDisplayName callback function
				getUserDisplayName := func(githubUsername string) string {
					if classroomErr == nil {
						if studentName, err := classroom.GetRepoName(githubUsername); err == nil && studentName != "" {
							return studentName
						}
					}
					return githubUsername
				}

				selectedCodespaces, err = ghapi.PromptForCodespaceSelection(codespaces, orgName, getUserDisplayName)
				if err != nil {
					mmc.Fatal(fmt.Errorf("failed to select codespaces: %v", err))
				}

				if len(selectedCodespaces) == 0 {
					fmt.Println("No codespaces selected for deletion.")
					return
				}
			}

			// Delete selected codespaces
			err = deleteSelectedCodespaces(client, orgName, selectedCodespaces, verbose)
			if err != nil {
				mmc.Fatal(fmt.Errorf("failed to delete selected codespaces: %v", err))
			}
		},
	}

	cmd.Flags().StringVarP(&orgName, "org", "o", "", "Organization name (if not provided, will be detected from classroom metadata or prompted)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose error output")
	cmd.Flags().BoolVarP(&all, "all", "a", false, "Delete all clean non-running codespaces (excludes those with uncommitted/unpushed changes)")

	return cmd
}

// deleteCodespace deletes a single codespace by name using the organization endpoint
func deleteCodespace(client *api.RESTClient, orgName, username, codespaceName string, verbose bool) error {
	// Use the organization codespace deletion endpoint
	endpoint := fmt.Sprintf("orgs/%s/members/%s/codespaces/%s", orgName, username, codespaceName)

	if verbose {
		fmt.Printf("DELETE %s\n", endpoint)
	}

	err := client.Delete(endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to delete codespace %s: %v", codespaceName, err)
	}

	return nil
}

// formatPrebuild formats prebuild availability information
func formatPrebuild(available bool, availability string) string {
	if available {
		return "✓"
	}

	switch availability {
	case "available":
		return "✓"
	case "in_progress":
		return "⏳"
	case "not_available":
		return "✗"
	default:
		return ""
	}
}

// formatGitStatus formats git status information for display
func formatGitStatus(gitStatus ghapi.GitHubCodespaceGitStatus) string {
	var status []string

	if gitStatus.HasUncommittedChanges {
		status = append(status, "U") // Uncommitted
	}
	if gitStatus.HasUnpushedChanges {
		status = append(status, "P") // Unpushed
	}

	if len(status) == 0 {
		return "✓" // Clean
	}

	return strings.Join(status, ",")
}

// deleteSelectedCodespaces deletes the specified codespaces
func deleteSelectedCodespaces(client *api.RESTClient, orgName string, codespaces []ghapi.GitHubCodespace, verbose bool) error {
	fmt.Printf("You selected %d codespace(s) for deletion.\n", len(codespaces))

	// Ask for confirmation
	fmt.Print("\nAre you sure you want to delete these codespaces? (y/N): ")
	var response string
	_, err := fmt.Scanln(&response)
	if err != nil {
		// Handle input error (e.g., EOF, interrupted input)
		fmt.Println("\nDeletion cancelled.")
		return nil
	}

	if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
		fmt.Println("Deletion cancelled.")
		return nil
	}

	// Delete each selected codespace
	fmt.Println("\nDeleting selected codespaces...")
	successCount := 0

	for _, cs := range codespaces {
		if verbose {
			fmt.Printf("Deleting codespace %s (%s)...\n", cs.DisplayName, cs.Name)
		}

		err := deleteCodespace(client, orgName, cs.Owner.Login, cs.Name, verbose)
		if err != nil {
			fmt.Printf("Failed to delete codespace %s: %v\n", cs.DisplayName, err)
			continue
		}

		successCount++
		if verbose {
			fmt.Printf("Successfully deleted codespace %s\n", cs.DisplayName)
		}
	}

	fmt.Printf("Deletion complete. Successfully deleted %d of %d codespaces.\n", successCount, len(codespaces))
	return nil
}

// displayCodespacesTable displays codespaces in a formatted table similar to the interactive selection
func displayCodespacesTable(codespaces []ghapi.GitHubCodespace, orgName string) {
	if len(codespaces) == 0 {
		return
	}

	// Load classroom context once for student name lookups
	classroom, classroomErr := mmc.LoadClassroom()

	// Calculate column widths for table alignment
	maxNameWidth := len("NAME")       // Start with header width
	maxRepoWidth := len("REPOSITORY") // Start with header width
	maxUserWidth := len("USER")       // Start with header width

	for _, cs := range codespaces {
		if len(cs.DisplayName) > maxNameWidth {
			maxNameWidth = len(cs.DisplayName)
		}

		// Strip organization prefix for width calculation
		repoDisplayName := cs.Repository.FullName
		if orgPrefix := orgName + "/"; strings.HasPrefix(repoDisplayName, orgPrefix) {
			repoDisplayName = repoDisplayName[len(orgPrefix):]
		}
		if len(repoDisplayName) > maxRepoWidth {
			maxRepoWidth = len(repoDisplayName)
		}

		// Check user display name length (student name or GitHub username)
		var displayUser string
		if classroomErr == nil {
			if name, err := classroom.GetRepoName(cs.Owner.Login); err == nil {
				displayUser = name
			}
		}
		if displayUser == "" {
			displayUser = cs.Owner.Login
		}
		if len(displayUser) > maxUserWidth {
			maxUserWidth = len(displayUser)
		}
	}

	// Print table header
	fmt.Printf("%-*s  %-6s  %-*s  %-*s  %-8s  %-5s  %s\n",
		maxNameWidth, "NAME",
		"GIT",
		maxRepoWidth, "REPOSITORY",
		maxUserWidth, "USER",
		"IDLE",
		"PRE",
		"LAST USED")

	// Print separator line
	fmt.Printf("%s  %s  %s  %s  %s  %s  %s\n",
		strings.Repeat("-", maxNameWidth),
		strings.Repeat("-", 6),
		strings.Repeat("-", maxRepoWidth),
		strings.Repeat("-", maxUserWidth),
		strings.Repeat("-", 8),
		strings.Repeat("-", 5),
		strings.Repeat("-", 19)) // Length of "Mon 2006-01-02 15:04"	// Print each codespace row
	for _, cs := range codespaces {
		// Get user display name (student name if available, otherwise GitHub username)
		var displayUser string
		if classroomErr == nil {
			if name, err := classroom.GetRepoName(cs.Owner.Login); err == nil {
				displayUser = name
			}
		}
		if displayUser == "" {
			displayUser = cs.Owner.Login
		}

		// Format last used time
		lastUsed := "Never"
		if cs.LastUsedAt != nil && *cs.LastUsedAt != "" {
			if t, err := time.Parse(time.RFC3339, *cs.LastUsedAt); err == nil {
				lastUsed = t.Format("Mon 2006-01-02 15:04")
			}
		}

		// Format idle timeout
		idleTimeout := fmt.Sprintf("%dm", cs.IdleTimeoutMinutes)

		// Format git status
		gitStatus := formatGitStatus(cs.GitStatus)

		// Strip organization prefix from repository name
		repoDisplayName := cs.Repository.FullName
		if orgPrefix := orgName + "/"; strings.HasPrefix(repoDisplayName, orgPrefix) {
			repoDisplayName = repoDisplayName[len(orgPrefix):]
		}

		// Handle nullable PrebuildAvailability
		var availability string
		if cs.Machine.PrebuildAvailability != nil {
			availability = *cs.Machine.PrebuildAvailability
		}
		prebuildInfo := formatPrebuild(cs.Prebuild, availability)

		// Print formatted row
		fmt.Printf("%-*s  %-6s  %-*s  %-*s  %-8s  %-5s  %s\n",
			maxNameWidth, cs.DisplayName,
			gitStatus,
			maxRepoWidth, repoDisplayName,
			maxUserWidth, displayUser,
			idleTimeout,
			prebuildInfo,
			lastUsed)
	}
	fmt.Println() // Add blank line after table
}
