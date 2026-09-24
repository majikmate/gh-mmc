package deletion

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/majikmate/gh-mmc/pkg/ghapi"
	"github.com/majikmate/gh-mmc/pkg/mmc"
	"github.com/spf13/cobra"
)

// course is the course of the current folder
type course struct {
	folder    string
	name      string
	classroom string
	org       string
	// repositories are the student repositories of the students on the roster and the starter repository last
	repositories []repository
}

// repository is a repository of a course
type repository struct {
	// folderName is the name of the local clone in the course folder, i.e. lastname.firstname, or the name of the
	// classroom for the starter repository
	folderName string
	fullName   string
	isStarter  bool
}

// target is a repository of the course to clean or delete
type target struct {
	// name is the name of the local clone, i.e. lastname.firstname, or the name of the classroom for the starter
	// repository
	name string
	url  string
	// remote is the repository on GitHub, if it exists
	remote ghapi.GithubRepository
	// folder is the local clone of the repository, if it exists
	folder       string
	localChanges bool
	isStarter    bool
	// starter is the starter repository of a fork that is deleted with it, i.e. without a request of its own
	starter *target
	err     error
}

func NewCmdClean(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Removes the local clones of all repositories of a course",
		Long: heredoc.Doc(`

			Removes the local clones of the starter repository and of the student repositories
			of a course. The repositories on GitHub are not changed, so gh mmc pull clones them
			again.

			The command must be run within a course folder, i.e. the folder containing the .mmc
			folder with the course.json file or any folder below it. It always operates in the
			course folder and returns to the current folder afterwards. If there is no course
			folder, it aborts with an error.

			The local clones of a course are the folder named after the classroom with the
			starter repository and the folders lastname.firstname with the student repositories
			of the students on the roster. Folders are only removed if they are a clone of the
			repository. Local clones with uncommitted or unpushed changes are kept, so that no
			work is lost. The metadata of the course is kept as well.`),
		Example: `$ gh mmc clean`,
		Run: func(cmd *cobra.Command, args []string) {
			clean()
		},
	}

	return cmd
}

func NewCmdDelete(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Deletes all repositories of a course on GitHub and locally",
		Long: heredoc.Doc(`

			Deletes the starter repository and the student repositories of a course, on GitHub
			and locally. This cannot be undone: deleted student repositories cannot be restored
			on GitHub, since they are forks of the starter repository.

			The command must be run within a course folder, i.e. the folder containing the .mmc
			folder with the course.json file or any folder below it. It always operates in the
			course folder and returns to the current folder afterwards. If there is no course
			folder, it aborts with an error.

			The repositories of a course are the starter repository of its course.json file and
			the student repositories <classroom>-<course>-<github user> of the students on the
			roster. Deleting the private starter repository on GitHub deletes all its forks as
			well, e.g. the ones of students that are not on the roster anymore. The template
			repository is not deleted. Local folders are only deleted if they are a clone of the
			repository. The metadata of the course is kept, so that gh mmc pull can set up the
			course again.

			The command lists all repositories it deletes, including local clones with changes
			that are not pushed, and asks to type the name of the classroom to confirm. Deleting
			repositories on GitHub requires the gh token to have the delete_repo scope. If it
			does not have it, the scope is added for deleting and removed again afterwards, both
			requiring to authenticate in the browser. A local clone is only deleted after its
			repository on GitHub was deleted.`),
		Example: `$ gh mmc delete`,
		Run: func(cmd *cobra.Command, args []string) {
			deleteCourse()
		},
	}

	return cmd
}

// openCourse changes to the course root of the current folder and returns the course and a function that changes
// back to the current folder
func openCourse() (course, func()) {
	courseFolder, err := mmc.FindCourseFolder()
	if err != nil {
		mmc.Fatal(err)
	}
	restore, err := mmc.ChangeToFolder(courseFolder)
	if err != nil {
		mmc.Fatal(err)
	}

	c, err := mmc.LoadClassroom()
	if err != nil {
		mmc.Fatal(err)
	}
	crs, err := mmc.LoadCourse()
	if err != nil {
		mmc.Fatal(err)
	}
	if c.Organization.Login == "" {
		mmc.Fatal(errors.New("no organization found in classroom: run `gh mmc init` to select the organization"))
	}
	starter, err := ghapi.FullNameFromURL(crs.StarterRepository)
	if err != nil {
		mmc.Fatal(fmt.Errorf("invalid starter repository of course %s: %v", crs.Name, err))
	}

	co := course{folder: courseFolder, name: crs.Name, classroom: c.Classroom.Name, org: c.Organization.Login}
	for _, s := range c.Students {
		name := mmc.StudentRepositoryName(co.classroom, co.name, s.GithubUser)
		co.repositories = append(co.repositories, repository{folderName: s.FolderName(), fullName: co.org + "/" + name})
	}
	co.repositories = append(co.repositories, repository{folderName: co.classroom, fullName: starter, isStarter: true})

	return co, restore
}

// localClone sets the local clone of the target, if the folder of the repository is a clone of it
func (co course) localClone(t *target, r repository) {
	if r.folderName == "" {
		return
	}
	folder := filepath.Join(co.folder, r.folderName)
	if mmc.IsCloneOf(folder, r.fullName) {
		t.folder = folder
		t.localChanges = mmc.HasLocalChanges(folder)
	}
}

// clean removes the local clones of the repositories of the course of the current folder
func clean() {
	co, restore := openCourse()
	defer restore()

	removed, kept, failed := 0, 0, 0
	for _, r := range co.repositories {
		t := &target{name: r.folderName, url: ghapi.RepositoryURL(r.fullName)}
		co.localClone(t, r)
		switch {
		case t.folder == "":
			continue
		case t.localChanges:
			fmt.Printf("Kept: %s (%s): uncommitted or unpushed changes\n", t.name, mmc.Cyan(t.url))
			kept++
		default:
			if err := os.RemoveAll(t.folder); err != nil {
				fmt.Printf("Failed: %s (%s): %v\n", t.name, mmc.Cyan(t.url), err)
				failed++
				continue
			}
			fmt.Printf("Removed: %s (%s)\n", t.name, mmc.Cyan(t.url))
			removed++
		}
	}

	if removed+kept+failed == 0 {
		fmt.Printf("No local clones of course %s found.\n", co.name)
		return
	}
	printTable(co, "Local clones", [][]string{
		{"Removed", strconv.Itoa(removed), "local clone removed"},
		{"Kept", strconv.Itoa(kept), "uncommitted or unpushed changes"},
		{"Failed", strconv.Itoa(failed), "failed, see above"},
	}, removed+kept+failed)

	if failed > 0 {
		os.Exit(1)
	}
}

// deleteCourse deletes the repositories of the course of the current folder on GitHub and locally after confirmation
func deleteCourse() {
	co, restore := openCourse()
	defer restore()

	client, err := api.DefaultRESTClient()
	if err != nil {
		mmc.Fatal(fmt.Errorf("failed to create gh client: %v", err))
	}

	// This is a destructive operation, so the name of the classroom must be typed to confirm it
	confirm := func(classroom string) (string, error) {
		return ghapi.PromptForName(fmt.Sprintf("Deleted repositories cannot be restored. Type the name of the classroom %s to delete them:", classroom), "", func(string) error {
			return nil
		})
	}

	failed, err := deleteRepositories(client, co, confirm)
	if err != nil {
		mmc.Fatal(err)
	}
	if failed {
		os.Exit(1)
	}
}

// deleteRepositories deletes the repositories of the course on GitHub and locally, if confirm returns the name of the
// classroom. It returns whether deleting any repository failed, and an error if nothing was deleted.
func deleteRepositories(client *api.RESTClient, co course, confirm func(classroom string) (string, error)) (bool, error) {
	repositories, err := ghapi.ListAllOrganizationRepositories(client, co.org)
	if err != nil {
		return false, fmt.Errorf("failed to list repositories of organization %s: %v", co.org, err)
	}
	existing := make(map[string]ghapi.GithubRepository)
	for _, r := range repositories {
		existing[strings.ToLower(r.FullName)] = r
	}

	// The student repositories are deleted before the starter repository, which deletes its remaining forks
	targets := []*target{}
	requested := make(map[string]bool)
	for _, r := range co.repositories {
		t := &target{name: r.folderName, url: ghapi.RepositoryURL(r.fullName), isStarter: r.isStarter}
		if remote, ok := existing[strings.ToLower(r.fullName)]; ok {
			t.remote = remote
		} else if owner, _, _ := strings.Cut(r.fullName, "/"); !strings.EqualFold(owner, co.org) {
			// The starter repository may belong to another organization
			remote, err := ghapi.GetRepository(client, r.fullName)
			if err != nil && !ghapi.IsNotFound(err) {
				return false, fmt.Errorf("failed to get repository %s: %v", r.fullName, err)
			}
			t.remote = remote
		}
		if t.remote.HtmlUrl != "" {
			t.url = t.remote.HtmlUrl
		}
		co.localClone(t, r)
		if t.remote.FullName == "" && t.folder == "" {
			continue
		}

		// The remaining forks of the starter repository on GitHub are deleted with it
		if r.isStarter && t.remote.FullName != "" {
			forks, err := ghapi.ListForks(client, t.remote)
			if err != nil {
				return false, fmt.Errorf("failed to list forks of %s: %v", t.remote.FullName, err)
			}
			for _, fork := range forks {
				if !requested[strings.ToLower(fork.FullName)] {
					targets = append(targets, &target{name: fork.Name, url: fork.HtmlUrl, remote: fork, starter: t})
				}
			}
		}
		targets = append(targets, t)
		requested[strings.ToLower(r.fullName)] = true
	}

	if len(targets) == 0 {
		fmt.Printf("No repositories of course %s found.\n", co.name)
		return false, nil
	}

	fmt.Printf("Repositories of course %s to delete:\n", co.name)
	for _, t := range targets {
		where := []string{}
		switch {
		case t.starter != nil:
			where = append(where, "GitHub, deleted with the starter repository")
		case t.remote.FullName != "":
			where = append(where, "GitHub")
		}
		switch {
		case t.folder != "" && t.localChanges:
			where = append(where, "local with uncommitted or unpushed changes")
		case t.folder != "":
			where = append(where, "local")
		}
		fmt.Printf("  %s (%s): %s\n", t.name, mmc.Cyan(t.url), strings.Join(where, ", "))
	}
	fmt.Println()

	name, err := confirm(co.classroom)
	if err != nil {
		return false, fmt.Errorf("nothing was deleted: %v", err)
	}
	if name != co.classroom {
		return false, fmt.Errorf("nothing was deleted: %q is not the name of the classroom %s", name, co.classroom)
	}

	// Repositories are deleted on GitHub first
	remote := []*target{}
	for _, t := range targets {
		if t.remote.FullName != "" && t.starter == nil {
			remote = append(remote, t)
		}
	}
	if len(remote) > 0 {
		deleting := false
		_, err = ghapi.WithScope(client, "delete_repo", func(client *api.RESTClient) error {
			deleting = true
			failed := false
			for _, t := range remote {
				// Deleting the starter repository would delete the student repositories that could not be deleted
				if t.isStarter && failed {
					t.err = errors.New("not deleted, since deleting student repositories failed, which it would delete as well")
					continue
				}
				err := ghapi.DeleteRepository(client, t.remote)
				// A fork may have been deleted with its starter repository already
				if err != nil && !ghapi.IsNotFound(err) {
					t.err = fmt.Errorf("failed to delete it on GitHub: %v", err)
					failed = true
				}
			}
			return nil
		})
		if err != nil && !deleting {
			for _, t := range remote {
				t.err = err
			}
		}
	}

	// Local clones are only deleted if their repository on GitHub was deleted
	deleted, failed := 0, 0
	for _, t := range targets {
		if t.starter != nil && t.starter.err != nil {
			t.err = errors.New("not deleted, since deleting the starter repository failed")
		}
		if t.err == nil && t.folder != "" {
			if err := os.RemoveAll(t.folder); err != nil {
				t.err = fmt.Errorf("failed to delete the local clone %s: %v", t.folder, err)
			}
		}

		if t.err != nil {
			fmt.Printf("Failed: %s (%s): %v\n", t.name, mmc.Cyan(t.url), t.err)
			failed++
		} else {
			fmt.Printf("Deleted: %s (%s)\n", t.name, mmc.Cyan(t.url))
			deleted++
		}
	}

	printTable(co, "Repositories", [][]string{
		{"Deleted", strconv.Itoa(deleted), "deleted on GitHub and locally"},
		{"Failed", strconv.Itoa(failed), "failed, see above"},
	}, len(targets))

	return failed > 0, nil
}

// printTable prints a summary of the repositories of the course, omitting the rows without repositories
func printTable(co course, what string, rows [][]string, total int) {
	nonEmpty := [][]string{}
	for _, row := range rows {
		if row[1] != "0" {
			nonEmpty = append(nonEmpty, row)
		}
	}

	mmc.Table{
		Title:        fmt.Sprintf("Course %s in organization %s:", co.name, co.org),
		Header:       []string{"Status", what, "Meaning"},
		Rows:         nonEmpty,
		Footer:       []string{"Total", strconv.Itoa(total), ""},
		RightAligned: map[int]bool{1: true},
	}.Print()
}
