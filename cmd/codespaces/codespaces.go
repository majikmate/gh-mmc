package codespaces

import (
	"errors"
	"fmt"
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

			fmt.Printf("Fetching codespaces for organization: %s\n\n", orgName)

			// Get codespaces for the organization
			codespaces, err := ghapi.GetCodespacesForOrg(client, orgName)
			if err != nil {
				mmc.Fatal(fmt.Errorf("failed to get codespaces: %v", err))
			}

			if len(codespaces) == 0 {
				fmt.Printf("No codespaces found for organization %s\n", orgName)
				return
			}

			// Print header with fixed-width formatting to handle emoji alignment
			fmt.Printf("%-25s %-5s %-35s %-20s %-42s %-8s %-8s %s\n",
				"NAME", "STATE", "REPOSITORY", "OWNER", "MACHINE", "PREBUILD", "IDLE", "LAST USED")

			for _, cs := range codespaces {
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

				// Use single-character emoji state indicator for proper alignment
				stateIndicator := ghapi.GetStateIndicator(cs.State)

				// Truncate long names and repositories for better formatting
				displayName := cs.DisplayName
				if len(displayName) > 24 {
					displayName = displayName[:21] + "..."
				}

				repoName := cs.Repository.FullName
				if len(repoName) > 34 {
					repoName = repoName[:31] + "..."
				}

				// Format idle timeout
				idleTimeout := fmt.Sprintf("%dm", cs.IdleTimeoutMinutes)

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

				fmt.Printf("%s%-25s %-5s %-35s %-20s %-42s %-8s %-8s %s%s\n",
					colorStart,
					displayName,
					stateIndicator,
					repoName,
					cs.Owner.Login,
					machineInfo,
					prebuildInfo,
					idleTimeout,
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

			This command will show you all available codespaces and allow you to 
			select which ones to delete. You can select multiple codespaces at once.

			Use the --all flag to automatically delete all non-running codespaces 
			without interactive selection.

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

			fmt.Printf("Fetching codespaces for organization: %s\n\n", orgName)

			// Get codespaces for the organization
			codespaces, err := ghapi.GetCodespacesForOrg(client, orgName)
			if err != nil {
				mmc.Fatal(fmt.Errorf("failed to get codespaces: %v", err))
			}

			if len(codespaces) == 0 {
				fmt.Printf("No codespaces found for organization %s\n", orgName)
				return
			}

			var selectedCodespaces []ghapi.GitHubCodespace

			if all {
				// Filter non-running codespaces when using --all flag
				for _, cs := range codespaces {
					if cs.State != "Available" {
						selectedCodespaces = append(selectedCodespaces, cs)
					}
				}

				if len(selectedCodespaces) == 0 {
					fmt.Println("No non-running codespaces found to delete.")
					return
				}

				fmt.Printf("Found %d non-running codespace(s) to delete with --all flag:\n\n", len(selectedCodespaces))
				
				// Display in table format similar to interactive selection
				displayCodespacesTable(selectedCodespaces)
			} else {
				// Prompt user to select codespaces to delete
				var err error
				selectedCodespaces, err = ghapi.PromptForCodespaceSelection(codespaces)
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
	cmd.Flags().BoolVarP(&all, "all", "a", false, "Delete all non-running codespaces without interactive selection")

	return cmd
}

// deleteNonRunningCodespaces deletes all codespaces that are not in "Available" state
func deleteNonRunningCodespaces(client *api.RESTClient, orgName string, codespaces []ghapi.GitHubCodespace, verbose bool) error {
	nonRunningCodespaces := []ghapi.GitHubCodespace{}

	// Filter for non-running codespaces
	for _, cs := range codespaces {
		if cs.State != "Available" {
			nonRunningCodespaces = append(nonRunningCodespaces, cs)
		}
	}

	if len(nonRunningCodespaces) == 0 {
		fmt.Println("No non-running codespaces found to delete.")
		return nil
	}

	fmt.Printf("Found %d non-running codespaces to delete:\n", len(nonRunningCodespaces))
	for _, cs := range nonRunningCodespaces {
		fmt.Printf("  - %s (%s) - State: %s\n", cs.DisplayName, cs.Repository.FullName, cs.State)
	}

	// Ask for confirmation
	fmt.Print("\nAre you sure you want to delete these codespaces? (y/N): ")
	var response string
	fmt.Scanln(&response)

	if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
		fmt.Println("Deletion cancelled.")
		return nil
	}

	// Delete each non-running codespace
	fmt.Println("\nDeleting non-running codespaces...")
	for _, cs := range nonRunningCodespaces {
		if verbose {
			fmt.Printf("Deleting codespace %s (%s)...\n", cs.DisplayName, cs.Name)
		}

		err := deleteCodespace(client, orgName, cs.Owner.Login, cs.Name, verbose)
		if err != nil {
			fmt.Printf("Failed to delete codespace %s: %v\n", cs.DisplayName, err)
			continue
		}

		if verbose {
			fmt.Printf("Successfully deleted codespace %s\n", cs.DisplayName)
		}
	}

	fmt.Printf("Deletion complete. Deleted %d codespaces.\n", len(nonRunningCodespaces))
	return nil
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

// deleteSelectedCodespaces deletes the specified codespaces
func deleteSelectedCodespaces(client *api.RESTClient, orgName string, codespaces []ghapi.GitHubCodespace, verbose bool) error {
	fmt.Printf("You selected %d codespace(s) for deletion.\n", len(codespaces))

	// Ask for confirmation
	fmt.Print("\nAre you sure you want to delete these codespaces? (y/N): ")
	var response string
	fmt.Scanln(&response)

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
func displayCodespacesTable(codespaces []ghapi.GitHubCodespace) {
	if len(codespaces) == 0 {
		return
	}

	// Calculate column widths for table alignment
	maxNameWidth := len("NAME")       // Start with header width
	maxRepoWidth := len("REPOSITORY") // Start with header width
	
	for _, cs := range codespaces {
		if len(cs.DisplayName) > maxNameWidth {
			maxNameWidth = len(cs.DisplayName)
		}
		if len(cs.Repository.FullName) > maxRepoWidth {
			maxRepoWidth = len(cs.Repository.FullName)
		}
	}

	// Print table header
	fmt.Printf("%-*s  %-*s  %-8s  %s\n",
		maxNameWidth, "NAME",
		maxRepoWidth, "REPOSITORY",
		"IDLE",
		"LAST USED")
	
	// Print separator line
	fmt.Printf("%s  %s  %s  %s\n",
		strings.Repeat("-", maxNameWidth),
		strings.Repeat("-", maxRepoWidth),
		strings.Repeat("-", 8),
		strings.Repeat("-", 19)) // Length of "Mon 2006-01-02 15:04"

	// Print each codespace row
	for _, cs := range codespaces {
		// Format last used time
		lastUsed := "Never"
		if cs.LastUsedAt != nil && *cs.LastUsedAt != "" {
			if t, err := time.Parse(time.RFC3339, *cs.LastUsedAt); err == nil {
				lastUsed = t.Format("Mon 2006-01-02 15:04")
			}
		}

		// Format idle timeout
		idleTimeout := fmt.Sprintf("%dm", cs.IdleTimeoutMinutes)

		// Print formatted row
		fmt.Printf("%-*s  %-*s  %-8s  %s\n",
			maxNameWidth, cs.DisplayName,
			maxRepoWidth, cs.Repository.FullName,
			idleTimeout,
			lastUsed)
	}
	fmt.Println() // Add blank line after table
}
