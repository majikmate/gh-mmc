package pull

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/go-gh/v2"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/majikmate/gh-mmc/pkg/ghapi"
	"github.com/majikmate/gh-mmc/pkg/mmc"
	"github.com/spf13/cobra"
)

const (
	// maxRepoNameLength is the maximum length of a GitHub repository name
	maxRepoNameLength = 100

	// contentTimeout is the time to wait for a repository created from a template or forked to be populated
	contentTimeout = 60 * time.Second
)

// repoNamePattern matches the characters GitHub allows in repository names
var repoNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// student is the state of a student of the course
type student struct {
	name       string
	githubUser string
	email      string
	// folder is the folder of the local clone of the student repository in the course folder
	folder string
	// repoName is the name of the student repository
	repoName   string
	membership ghapi.MembershipStatus
	// repository is the student repository on GitHub, if it exists
	repository     ghapi.GithubRepository
	hasWriteAccess bool
	created        bool
	// changes are the changes made to an existing student repository on GitHub, e.g. made private
	changes []string
	// local is the action taken for the local clone, i.e. cloned, pulled or clean
	local string
	// synced is set if changes of the starter repository were merged into the student repository
	synced bool
	// invalid is the reason why the student repository on GitHub is invalid
	invalid error
	// errors are the errors that occurred for the student
	errors []string
}

// String returns the local folder and the URL of the student repository, i.e.
// lastname.firstname (https://github.com/org/repository), or the GitHub user of the student if there is no student
// repository
func (s *student) String() string {
	if s.repository.HtmlUrl == "" {
		return fmt.Sprintf("%s (%s)", s.folder, s.githubUser)
	}
	return fmt.Sprintf("%s (%s)", s.folder, mmc.Cyan(s.repository.HtmlUrl))
}

// fail records an error of the student
func (s *student) fail(err error) {
	s.errors = append(s.errors, err.Error())
}

// line returns the line of the student in the list of repositories, labeled with the most important outcome
func (s *student) line() string {
	switch {
	case s.invalid != nil:
		return fmt.Sprintf("%s: %s: %v", labelInvalid, s, s.invalid)
	case len(s.errors) > 0:
		return fmt.Sprintf("%s: %s: %s", labelFailed, s, strings.Join(s.errors, "; "))
	case s.membership == ghapi.MembershipInvited || s.membership == ghapi.MembershipPending:
		return fmt.Sprintf("%s: %s", labelPending, s)
	case s.created:
		return fmt.Sprintf("%s: %s", labelCreated, s)
	case len(s.changes) > 0:
		details := slices.Clone(s.changes)
		if s.synced {
			details = append(details, "synced")
		}
		return fmt.Sprintf("%s: %s, %s", labelUpdated, s, strings.Join(details, ", "))
	case s.synced:
		return fmt.Sprintf("%s: %s", labelSynced, s)
	case s.local == "":
		return fmt.Sprintf("%s: %s", actionClean, s)
	}
	return fmt.Sprintf("%s: %s", s.local, s)
}

// status returns the status of the student in the course
func (s *student) status() string {
	switch {
	case s.invalid != nil:
		return statusInvalid
	case len(s.errors) > 0:
		return statusFailed
	case s.membership == ghapi.MembershipInvited:
		return statusInvited
	case s.membership == ghapi.MembershipPending:
		return statusPending
	case s.created:
		return statusCreated
	}
	return statusExisting
}

func NewCmdPull(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Creates a course, or sets up, clones and pulls the repositories of a course",
		Long: heredoc.Doc(`

			Creates a course, or sets up the repositories of a course on GitHub, and clones and
			pulls them.

			The command can be run in a course folder, in a classroom folder or in any folder
			below them. It searches the course root first, i.e. the folder containing the .mmc
			folder with the course.json file, and then the classroom root, i.e. the folder
			containing the .mmc folder with the classroom.json file. If it finds neither, it
			aborts with an error. It always operates in the root it found and returns to the
			current folder afterwards.

			Run within a classroom outside of a course, the command creates a new course. The
			user will be prompted to select a template repository from all template
			repositories in all organizations the user is a member of, and to enter the name of
			the course. The name of the template repository is proposed as course name. The
			course folder <course> is created in the classroom folder, with the file
			<course>/.mmc/course.json containing the URLs of the template repository and of the
			starter repository <classroom>-<course> in the organization of the classroom.

			Run within a course, the command sets up the course with the metadata of the
			course.json file. The name of the course folder is the name of the course.

			The starter repository is created from the template repository unless it exists
			already. An existing starter repository must have been created from the template
			repository. The starter repository is always private, whether the template
			repository is public or private, so that the student repositories forked from it
			are private as well. An existing starter or student repository that is not private
			is made private.

			For every student in the classroom, the starter repository is forked into the
			organization of the classroom as student repository
			<classroom>-<course>-<github user>, and <github user> is granted write access to
			it. Students must be members of the organization of the classroom. Students that
			are not are invited to the organization, unless an invitation is pending already.
			Their student repositories cannot be created before they accepted the invitation:
			run the command again afterwards.
			Inviting requires the user to be an owner of the organization and the gh token to
			have the admin:org scope. If it does not have it, the scope is added for inviting
			and removed again afterwards, both requiring to authenticate in the browser.

			There is only one student repository per student in a course, named after the
			GitHub user of the student. A student repository on GitHub is invalid if its
			student is not a member of the organization, or if any other user than the logged
			in user and the organization owners has write access to it. It is invalid as well
			if forking the starter repository returns another repository than the student
			repository, e.g. because its name was taken in the meantime. Invalid student
			repositories are only reported: they are neither changed on GitHub, nor cloned or
			pulled. Fix them on GitHub. A valid student repository that exists already is left
			as is if the student has write access to it already. Repositories are never
			deleted, neither on GitHub nor locally.

			The starter repository is always cloned into a folder named after the classroom.
			Every student repository is cloned into the folder lastname.firstname of the course
			folder, made from the part of the email of the student before the @, which is
			firstname.lastname. Repositories that are cloned already are pulled from their
			default branch. Local changes are stashed before pulling and restored afterwards.
			Every valid student repository that is available on GitHub is cloned or pulled,
			even if it could not be set up completely.

			The command lists the starter repository first and then every student once, by the
			local folder and the URL of the repository, e.g.
			Pulled: lastname.firstname (https://github.com/org/repo). The label tells what
			happened: Created, Updated (made private or write access granted), Synced (gh mmc
			sync only), Cloned, Pulled, or Clean if nothing changed, or Pending if the invitation
			of the student is not accepted yet, Invalid or Failed. At the end, a table summarizes
			the status of the students in the course.`),
		Example: `$ gh mmc pull`,
		Run: func(cmd *cobra.Command, args []string) {
			Run(Options{})
		},
	}

	return cmd
}

// Options control what Run does besides pulling
type Options struct {
	// Sync requires a course, and synchronizes the student repositories with the starter repository after pulling
	Sync bool
}

// Run creates a new course in the classroom root, or sets up the course of the course root, and clones and pulls
// the repositories of the course
func Run(opts Options) {
	// The course root is searched first, then the classroom root
	courseFolder, err := mmc.FindCourseFolder()
	isNewCourse := errors.Is(err, mmc.ErrCourseNotFound)
	if err != nil && !isNewCourse {
		mmc.Fatal(err)
	}
	if isNewCourse && opts.Sync {
		mmc.Fatal(err)
	}

	classroomFolder, err := mmc.FindClassroomFolder()
	if err != nil {
		if isNewCourse && errors.Is(err, mmc.ErrClassroomNotFound) {
			err = errors.New("no course or classroom found: run `gh mmc init` to create a classroom, or change to a classroom or course folder")
		}
		mmc.Fatal(err)
	}

	// Execute in the course root, or in the classroom root for a new course, and return to the current folder
	// afterwards
	root := courseFolder
	if isNewCourse {
		root = classroomFolder
	}
	restore, err := mmc.ChangeToFolder(root)
	if err != nil {
		mmc.Fatal(err)
	}
	defer restore()

	c, err := mmc.LoadClassroom()
	if err != nil {
		mmc.Fatal(err)
	}

	org := c.Organization.Login
	if org == "" {
		mmc.Fatal(errors.New("no organization found in classroom: run `gh mmc init` to select the organization"))
	}

	classroom := c.Classroom.Name
	if !repoNamePattern.MatchString(classroom) {
		mmc.Fatal(fmt.Errorf("classroom name %s may only contain letters, digits, '-', '_' and '.': rename the classroom folder", classroom))
	}

	if len(c.Students) == 0 {
		mmc.Fatal(errors.New("no students found in classroom: run `gh mmc init` to read the accounts"))
	}
	for _, s := range c.Students {
		if !repoNamePattern.MatchString(s.GithubUser) {
			mmc.Fatal(fmt.Errorf("student %s has an invalid GitHub user %q: fix the accounts file and run `gh mmc init`", s.Name, s.GithubUser))
		}
	}

	client, err := api.DefaultRESTClient()
	if err != nil {
		mmc.Fatal(fmt.Errorf("failed to create gh client: %v", err))
	}

	// The URLs of the template repository and of the starter repository of the course
	var course, templateURL, starterURL string
	if isNewCourse {
		fmt.Println("Searching template repositories...")
		templates, err := ghapi.ListTemplateRepositories(client)
		if err != nil {
			mmc.Fatal(fmt.Errorf("failed to list template repositories: %v", err))
		}

		template, err := ghapi.PromptForTemplateRepository(templates)
		if err != nil {
			mmc.Fatal(fmt.Errorf("failed to get template repository: %v", err))
		}

		course, err = ghapi.PromptForName("Name of the course:", template.Name, func(course string) error {
			if !repoNamePattern.MatchString(course) || strings.HasPrefix(course, ".") {
				return errors.New("the course name may only contain letters, digits, '-', '_' and '.' and must not start with '.'")
			}
			for _, s := range c.Students {
				if name := mmc.StudentRepositoryName(classroom, course, s.GithubUser); len(name) > maxRepoNameLength {
					return fmt.Errorf("the repository name %s is longer than %d characters", name, maxRepoNameLength)
				}
			}
			if _, err := os.Stat(filepath.Join(classroomFolder, course)); err == nil {
				return fmt.Errorf("the folder %s exists already in the classroom folder", course)
			}
			return nil
		})
		if err != nil {
			mmc.Fatal(fmt.Errorf("failed to get course name: %v", err))
		}

		courseFolder = filepath.Join(classroomFolder, course)
		templateURL = template.HtmlUrl
		starterURL = ghapi.RepositoryURL(org + "/" + mmc.StarterRepositoryName(classroom, course))
	} else {
		crs, err := mmc.LoadCourse()
		if err != nil {
			mmc.Fatal(err)
		}
		course = crs.Name
		if !repoNamePattern.MatchString(course) {
			mmc.Fatal(fmt.Errorf("course name %s may only contain letters, digits, '-', '_' and '.': rename the course folder", course))
		}
		templateURL = crs.TemplateRepository
		starterURL = crs.StarterRepository
	}

	templateFullName, err := ghapi.FullNameFromURL(templateURL)
	if err != nil {
		mmc.Fatal(fmt.Errorf("invalid template repository of course %s: %v", course, err))
	}
	starterFullName, err := ghapi.FullNameFromURL(starterURL)
	if err != nil {
		mmc.Fatal(fmt.Errorf("invalid starter repository of course %s: %v", course, err))
	}

	// The logged in user and the organization owners may have write access to student repositories
	me, err := ghapi.GetCurrentUser(client)
	if err != nil {
		mmc.Fatal(fmt.Errorf("failed to get logged in user: %v", err))
	}
	owners, err := ghapi.ListOrganizationOwners(client, org)
	if err != nil {
		mmc.Fatal(fmt.Errorf("failed to list owners of organization %s: %v", org, err))
	}
	staff := map[string]bool{strings.ToLower(me.Login): true}
	for _, o := range owners {
		staff[strings.ToLower(o.Login)] = true
	}

	// Students must be members of the organization
	membership, err := ghapi.LoadOrganizationMembership(client, org)
	if err != nil {
		mmc.Fatal(err)
	}

	repositories, err := ghapi.ListAllOrganizationRepositories(client, org)
	if err != nil {
		mmc.Fatal(fmt.Errorf("failed to list repositories of organization %s: %v", org, err))
	}
	existing := make(map[string]ghapi.GithubRepository)
	for _, r := range repositories {
		existing[strings.ToLower(r.Name)] = r
	}

	// An existing starter repository must have been created from the template repository of the course,
	// e.g. by a previous run
	starter, err := ghapi.GetRepository(client, starterFullName)
	starterExists := err == nil
	if err != nil && !ghapi.IsNotFound(err) {
		mmc.Fatal(fmt.Errorf("failed to get starter repository %s: %v", starterURL, err))
	}
	if starterExists && (starter.TemplateRepository == nil || !strings.EqualFold(starter.TemplateRepository.FullName, templateFullName)) {
		mmc.Fatal(fmt.Errorf("starter repository %s exists already, but was not created from %s", starterURL, templateURL))
	}

	if isNewCourse {
		err = os.Mkdir(courseFolder, 0755)
		if err != nil {
			mmc.Fatal(fmt.Errorf("failed to create course folder %s: %v", courseFolder, err))
		}
		err = mmc.NewCourse(templateURL, starterURL).Save(courseFolder)
		if err != nil {
			mmc.Fatal(fmt.Errorf("failed to save course: %v", err))
		}
		fmt.Printf("Created course folder: %s\n", courseFolder)
	}

	starterChanges := []string{}
	if !starterExists {
		template, err := ghapi.GetRepository(client, templateFullName)
		if err != nil {
			mmc.Fatal(fmt.Errorf("failed to get template repository %s: %v", templateURL, err))
		}
		starter, err = ghapi.CreateRepositoryFromTemplate(client, template, starterFullName)
		if err != nil {
			mmc.Fatal(fmt.Errorf("failed to create starter repository %s from %s: %v", starterURL, templateURL, err))
		}
		starterChanges = append(starterChanges, starterCreated)
	}

	// The starter repository must be private, whatever the visibility of the template repository, so that
	// the student repositories forked from it are private as well
	if !starter.IsPrivate() {
		private, err := ghapi.SetRepositoryPrivate(client, starter)
		if err != nil {
			mmc.Fatal(fmt.Errorf("failed to make starter repository %s private: %v", starter.HtmlUrl, err))
		}
		starter = private
		starterChanges = append(starterChanges, "made private")
	}

	err = ghapi.WaitForRepositoryContent(client, starter, contentTimeout)
	if err != nil {
		mmc.Fatal(err)
	}

	// The starter repository is always cloned or pulled first, and is the first line of the list of repositories. Its
	// local folder is always named after the classroom.
	action, err := syncClone(client, starter, filepath.Join(courseFolder, classroom))
	starterLine := fmt.Sprintf("%s (%s)", classroom, mmc.Cyan(starter.HtmlUrl))
	failed := err != nil
	switch {
	case err != nil:
		fmt.Printf("%s: %s: %v\n", labelFailed, starterLine, err)
	case slices.Contains(starterChanges, starterCreated):
		fmt.Printf("%s: %s\n", labelCreated, starterLine)
	case len(starterChanges) > 0:
		fmt.Printf("%s: %s, %s\n", labelUpdated, starterLine, strings.Join(starterChanges, ", "))
	default:
		fmt.Printf("%s: %s\n", action, starterLine)
	}

	students := make([]*student, 0, len(c.Students))
	invitees := make([]ghapi.Invitee, 0, len(c.Students))
	for _, cs := range c.Students {
		name := mmc.StudentRepositoryName(classroom, course, cs.GithubUser)
		students = append(students, &student{
			name:       cs.Name,
			githubUser: cs.GithubUser,
			email:      cs.Email,
			folder:     cs.FolderName(),
			repoName:   name,
			repository: existing[strings.ToLower(name)],
		})
		invitees = append(invitees, ghapi.Invitee{Login: cs.GithubUser, Email: cs.Email})
	}

	// Students that are neither members nor invited are invited to the organization
	client, inviteErrs := membership.InviteMissing(client, invitees)
	for _, s := range students {
		s.membership = membership.Status(s.githubUser, s.email)
		if err := inviteErrs[strings.ToLower(s.githubUser)]; err != nil {
			s.fail(err)
		} else if s.membership == ghapi.MembershipNone {
			s.fail(fmt.Errorf("%s is not a member of organization %s", s.githubUser, org))
		}
	}

	// Every student is processed completely before the line of the student is printed, so that every student
	// appears only once in the list of repositories
	statuses := make([]string, 0, len(students))
	for _, s := range students {
		processStudent(client, s, org, starter, staff, courseFolder, opts.Sync)
		fmt.Println(s.line())
		statuses = append(statuses, s.status())
		failed = failed || s.invalid != nil || len(s.errors) > 0
	}

	printCourseTable(course, org, statuses)

	if failed {
		os.Exit(1)
	}
}

// Status of a student in a course
const (
	statusExisting = "Existing"
	statusCreated  = "Created"
	statusInvited  = "Invited"
	statusPending  = "Pending"
	statusInvalid  = "Invalid"
	statusFailed   = "Failed"
)

// printCourseTable prints an overview of the status of the students in the course, omitting the statuses no student
// has
func printCourseTable(course, org string, statuses []string) {
	count := make(map[string]int)
	for _, status := range statuses {
		count[status]++
	}

	rows := [][]string{}
	for _, status := range []struct{ name, meaning string }{
		{statusExisting, "repository existed already"},
		{statusCreated, "repository created now"},
		{statusInvited, "invited now, no repository yet"},
		{statusPending, "invitation pending, no repository yet"},
		{statusInvalid, "invalid repository on GitHub, see above"},
		{statusFailed, "failed, see above"},
	} {
		if count[status.name] > 0 {
			rows = append(rows, []string{status.name, strconv.Itoa(count[status.name]), status.meaning})
		}
	}

	mmc.Table{
		Title:        fmt.Sprintf("Course %s in organization %s:", course, org),
		Header:       []string{"Status", "Students", "Meaning"},
		Rows:         rows,
		Footer:       []string{"Total", strconv.Itoa(len(statuses)), ""},
		RightAligned: map[int]bool{1: true},
	}.Print()
}

// processStudent checks the student repository on GitHub and sets it up for a member of the organization. If sync is
// set, it synchronizes the student repository with the starter repository on GitHub. Then it clones or pulls it, so
// that its local clone has the latest state. Invalid student repositories are only reported, they are neither changed
// nor cloned or pulled.
func processStudent(client *api.RESTClient, s *student, org string, starter ghapi.GithubRepository, staff map[string]bool, courseFolder string, sync bool) {
	checkStudentRepository(client, s, org, staff)
	if s.invalid != nil || s.membership != ghapi.MembershipActive || len(s.errors) > 0 {
		return
	}

	exists := s.repository.FullName != ""
	repository, changes, err := setupStudentRepository(client, org, starter, s)
	if s.invalid != nil {
		return
	}
	s.repository = repository
	s.created = !exists && s.repository.FullName != ""
	s.changes = changes
	if err != nil {
		s.fail(err)
	}
	if s.repository.FullName == "" {
		return
	}

	// Student repositories are synchronized with the starter repository on GitHub before they are pulled, so that the
	// local clones have the latest state. Forks created in this run are up to date with the starter repository
	// already.
	if sync && !s.created {
		branch := s.repository.DefaultBranch
		if branch == "" {
			branch = "main"
		}
		mergeType, err := ghapi.MergeUpstream(client, s.repository, branch)
		if err != nil {
			s.fail(fmt.Errorf("failed to sync with the starter repository: %v", err))
		} else {
			s.synced = mergeType != syncUpToDate
		}
	}

	// Available student repositories are cloned or pulled, even if they could not be set up or synchronized completely
	action, err := syncClone(client, s.repository, filepath.Join(courseFolder, s.folder))
	if err != nil {
		s.fail(err)
	}
	s.local = action
}

// checkStudentRepository checks if the student repository on GitHub is valid. It is invalid if the student is not a
// member of the organization, or if any other user than the staff has write access to it. It also notes if the student
// has write access to it.
func checkStudentRepository(client *api.RESTClient, s *student, org string, staff map[string]bool) {
	if s.repository.FullName == "" {
		return
	}

	if s.membership != ghapi.MembershipActive {
		s.invalid = fmt.Errorf("the student repository exists, although %s is not a member of organization %s", s.githubUser, org)
		return
	}

	writers, err := ghapi.ListUsersWithWriteAccess(client, s.repository)
	if err != nil {
		s.fail(fmt.Errorf("failed to list users with write access: %v", err))
		return
	}

	others := []string{}
	for _, w := range writers {
		if strings.EqualFold(w, s.githubUser) {
			s.hasWriteAccess = true
		} else if !staff[strings.ToLower(w)] {
			others = append(others, w)
		}
	}
	if len(others) > 0 {
		s.invalid = fmt.Errorf("other users have write access: %s", strings.Join(others, ", "))
	}
}

// setupStudentRepository forks the starter repository into the student repository unless it exists already, makes
// sure it is private and grants the student write access to it unless the student has it already. It returns the
// student repository and the changes made to it. On errors, it returns the student repository as well if it exists.
// If GitHub returns another repository than the requested fork, the student repository is marked invalid, since
// there is only one student repository per student in a course.
func setupStudentRepository(client *api.RESTClient, org string, starter ghapi.GithubRepository, s *student) (ghapi.GithubRepository, []string, error) {
	repository := s.repository
	if repository.FullName == "" {
		fork, err := ghapi.CreateFork(client, starter, org, s.repoName)
		if err != nil {
			return ghapi.GithubRepository{}, nil, fmt.Errorf("failed to fork %s: %v", starter.FullName, err)
		}
		if !strings.EqualFold(fork.Name, s.repoName) {
			s.invalid = fmt.Errorf("forking returned the repository %s (%s) instead of the student repository, but there is only one student repository per student in a course", fork.Name, fork.HtmlUrl)
			return ghapi.GithubRepository{}, nil, nil
		}
		repository = fork
	}

	changes := []string{}

	// Student repositories must be private, e.g. if forked before the starter repository was made private
	if !repository.IsPrivate() {
		private, err := ghapi.SetRepositoryPrivate(client, repository)
		if err != nil {
			return repository, changes, fmt.Errorf("failed to make it private: %v", err)
		}
		repository = private
		changes = append(changes, "made private")
	}

	if !s.hasWriteAccess {
		err := ghapi.AddCollaborator(client, repository, s.githubUser, "push")
		if err != nil {
			return repository, changes, fmt.Errorf("failed to grant %s write access: %v", s.githubUser, err)
		}
		changes = append(changes, "granted "+s.githubUser+" write access")
	}

	return repository, changes, nil
}

// syncUpToDate is the merge type of a student repository that was up to date with the starter repository already
const syncUpToDate = "none"

// Actions taken for a local clone
const (
	actionCloned = "Cloned"
	actionPulled = "Pulled"
	actionClean  = "Clean"
)

// Labels of the lines in the list of repositories besides the actions taken for local clones
const (
	labelInvalid = "Invalid"
	labelFailed  = "Failed"
	labelPending = "Pending"
	labelCreated = "Created"
	labelUpdated = "Updated"
	labelSynced  = "Synced"
)

// starterCreated is the change of a starter repository created in this run
const starterCreated = "created"

// syncClone clones the repository into the folder unless it is cloned there already, and pulls it from its default
// branch otherwise. It fails if the folder exists, but is not a clone of the repository. It returns the action taken,
// which is clean if pulling did not change anything.
func syncClone(client *api.RESTClient, repository ghapi.GithubRepository, folder string) (string, error) {
	if _, err := os.Stat(folder); err == nil {
		if !isCloneOf(folder, repository) {
			return "", fmt.Errorf("folder %s exists, but is not a clone of %s", folder, repository.FullName)
		}
		before, _ := gitHead(folder)
		if err := pullRepository(folder, repository.DefaultBranch); err != nil {
			return "", err
		}
		if after, _ := gitHead(folder); before != "" && before == after {
			return actionClean, nil
		}
		return actionPulled, nil
	}

	// Forks are populated asynchronously
	err := ghapi.WaitForRepositoryContent(client, repository, contentTimeout)
	if err != nil {
		return "", err
	}

	_, stderr, err := gh.Exec("repo", "clone", repository.FullName, folder)
	if err != nil {
		return "", fmt.Errorf("failed to clone %s: %v: %s", repository.FullName, err, strings.TrimSpace(stderr.String()))
	}

	return actionCloned, nil
}

// gitHead returns the commit HEAD refers to in the git repository in the folder
func gitHead(folder string) (string, error) {
	out, err := exec.Command("git", "-C", folder, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// pullRepository safely pulls a repository using autostash to preserve local changes
// This ensures we get the latest content while preserving any uncommitted student work
func pullRepository(repoPath, defaultBranch string) error {
	if defaultBranch == "" {
		defaultBranch = "main" // fallback to main if not specified
	}

	// Pull with autostash - this handles fetch, merge, and stashing automatically
	pullCmd := exec.Command("git", "pull", "--autostash", "origin", defaultBranch)
	pullCmd.Dir = repoPath
	var pullOut bytes.Buffer
	pullCmd.Stdout = &pullOut
	pullCmd.Stderr = &pullOut
	if err := pullCmd.Run(); err != nil {
		return fmt.Errorf("git pull failed: %v\nOutput: %s", err, pullOut.String())
	}

	return nil
}

// isCloneOf checks if the folder is a git repository whose origin is the repository
func isCloneOf(folder string, repository ghapi.GithubRepository) bool {
	if _, err := os.Stat(filepath.Join(folder, ".git")); err != nil {
		return false
	}

	out, err := exec.Command("git", "-C", folder, "remote", "get-url", "origin").Output()
	if err != nil {
		return false
	}

	url := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(string(out)), ".git"))
	fullName := strings.ToLower(repository.FullName)
	return strings.HasSuffix(url, "/"+fullName) || strings.HasSuffix(url, ":"+fullName)
}
