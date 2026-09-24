[![Contributor Covenant](https://img.shields.io/badge/Contributor%20Covenant-2.1-4baaaa.svg)](CODE_OF_CONDUCT.md)

# gh mmc

This extension is an opinionated [GitHub CLI](https://cli.github.com) extension for teachers to manage the student repositories of their classes in a GitHub organization. It works directly with the organization and its repositories and does not use GitHub Classroom:

- `gh mmc init` creates the roster of a classroom from an Excel file and invites the students to the organization
- `gh mmc pull` creates a course from a template repository, i.e. a private starter repository and a student repository forked from it for every student, and clones and pulls them
- `gh mmc sync` distributes the changes of the starter repository to the student repositories
- `gh mmc check` compares the files of the students within each assignment to detect potential plagiarism
- `gh mmc codespaces` lists and removes the codespaces of the organization
- `gh mmc clean` and `gh mmc delete` remove the local clones of a course, or its repositories on GitHub and locally

# Installation
- Install the gh cli

  On MacOS, e.g., use [Homebrew](https://brew.sh/)

  ```bash
  brew install gh
  ```

- Authorize gh cli to access your GitHub

  ```bash
  gh auth login
  ```
  ... and follow the prompts on the commandline

- Install this extension
  ```bash
  gh extension install majikmate/gh-mmc
  ```

- Upgrade this extension
  ```bash
  gh extension upgrade mmc
  ```

- Remove this extension
  ```bash
  gh extension remove mmc
  ```

- List installed extensions
  ```bash
  gh extension list
  ```

# Usage

## Initialization

In order to start with the tool and initialize a classroom repository on the local file system, an Excel file containing a list of students with additional metadata is required in the local folder that should become the root of the local classroom repository.

The Excel file needs to contain a header line in the first row containing following fields:
- Name         ... Full name of the student
- Email        ... Email address of the student
- GitHub User  ... GitHub username of the student

The additional lines need to contain at least one line with the respective student information.

The Email should contain Emails of the students in the format
- *firstname*.*lastname*@domain.tld

The Excel file must be named with a prefix of *account* or *Account* and should have the file extension *.xlsx*. This file can be created, e.g., by gathering student details through a [Microsoft Office Forms](http://forms.office.com/) form and exporting the responses. A template can be downloaded from [Accounts](res/accounts.xltx).

`gh mmc init` creates or updates the roster of the classroom from the Excel file. When running it, you will be prompted to select the GitHub organization that hosts the student repositories from the list of organizations you are a member of. Then you will be prompted to select the organizations containing the template repositories that courses are created from, see [Courses](#courses). At least one organization must be selected. The organizations selected before are preselected, or for a new classroom, the organization of the classroom. The selected organizations are saved in `.mmc/classroom.json`. Students that are not members of the organization are invited to it, unless an invitation is pending already.

Inviting students (by `gh mmc init` and `gh mmc pull`) requires you to be an owner of the organization and the gh token to have the `admin:org` scope. If the token does not have it, the command adds the scope for inviting and removes it again afterwards (`gh auth refresh --scopes admin:org` and `gh auth refresh --remove-scopes admin:org`). Both require you to authenticate in the browser. If no student needs to be invited, the scope is not touched.

The folder containing the `.mmc` folder created by `gh mmc init` is the classroom folder. Its name is the name of the classroom.

## Root folders

The commands can be run in any folder below their root folder. They always operate in their root folder and return to the current folder afterwards:
- `gh mmc init` searches for the classroom root, i.e. the folder containing `.mmc/classroom.json`, or for a new classroom, the folder containing the accounts file. If it finds neither, it aborts with an error.
- `gh mmc pull` searches for the course root, i.e. the folder containing `.mmc/course.json`, and then for the classroom root. If it finds neither, it aborts with an error.
- `gh mmc sync`, `gh mmc check`, `gh mmc clean` and `gh mmc delete` search for the course root. If they do not find one, they abort with an error.
- `gh mmc codespaces` can be run in any folder. Within a course or a classroom, it only operates on the codespaces of their repositories, see [Codespaces](#codespaces).

## Courses

`gh mmc pull` run within a classroom, but outside of a course, creates a new course. You will be prompted to select a template repository from all template repositories that are not archived in the template organizations of the classroom, which are selected by `gh mmc init`, and to enter the name of the course. If the classroom has no template organizations yet, e.g. because it was created by an older version, run `gh mmc init` again to select them. The name of the template repository is proposed as course name.

The command then creates, in the organization of the classroom:
- the private starter repository *classroom*-*course* from the template repository, and
- a fork of the starter repository for every student as student repository *classroom*-*course*-*github user*. The student *github user* is granted write access to it. Students must be members of the organization of the classroom. Students that are not are invited to the organization, unless an invitation is pending already. Their student repositories cannot be created before they accepted the invitation: run the command again afterwards.

The starter repository is always private, whether the template repository is public or private, so that the student repositories forked from it are private as well. An existing starter or student repository that is not private is made private.

Right after the course name is entered, the command creates the course folder in the classroom folder. The name of the course folder is the name of the course. The file `course.json` in it contains the URLs of the template repository and of the starter repository, e.g.:

```json
{
    "TemplateRepository": "https://github.com/HTLD-STH-SWP/module-ts",
    "StarterRepository": "https://github.com/HTLD-STH-SWP/2025-4bWI-SWP-module-ts"
}
```

The starter repository is always cloned into a folder named after the classroom, and every student repository into a folder *lastname*.*firstname* of the course folder, made from the email *firstname*.*lastname*@domain.tld of the student:

```
classroom/.mmc/classroom.json
classroom/course/.mmc/course.json
classroom/course/classroom/.git
classroom/course/lastname.firstname/.git
```

`gh mmc pull` run within a course sets up that course again with the metadata of `course.json`, e.g., after adding students or after students accepted the invitation to the organization. The starter repository is created from the template repository if it does not exist yet, and missing student repositories are created. Valid student repositories that exist already are left as is if the student has write access already. Repositories that are not cloned yet are cloned, and repositories that are cloned already are pulled. Local changes are stashed before pulling and restored afterwards. Every valid student repository that is available on GitHub is cloned or pulled, even if it could not be set up completely.

There is only one student repository per student in a course, named after the GitHub user of the student. A student repository on GitHub is invalid if its student is not a member of the organization, or if any user other than you and the organization owners has write access to it. It is invalid as well if forking the starter repository returns another repository than the student repository, e.g., because its name was taken in the meantime. Invalid student repositories are only reported: they are neither changed on GitHub, nor cloned or pulled. Fix them on GitHub. `gh mmc` never deletes a repository, neither on GitHub nor locally.

## Syncing

`gh mmc sync` synchronizes the student repositories of a course with the starter repository on GitHub, so that the students can pull the changes of the starter repository, e.g., example code that shall be distributed to the students. It must be run within a course folder and always operates in the course folder. If there is no course folder, it aborts with an error.

The command does everything `gh mmc pull` does. Additionally, it synchronizes the default branch of every valid student repository on GitHub with the starter repository before pulling it, so that the local clones have the latest state. Student repositories created in the same run are up to date already. Student repositories whose changes conflict with the changes of the starter repository cannot be synchronized and are reported as an error. Invalid student repositories are only reported and not synchronized.

## Checking for plagiarism

`gh mmc check` compares the submissions of the students of a course to detect potential plagiarism. It compares the local clones of the student repositories, so run `gh mmc pull` first. Students whose repository is not cloned are listed and skipped. The starter repository is not compared.

Every folder in the folder `20-assignments` of a student repository is an assignment:

```
classroom/course/lastname.firstname/20-assignments/assignment
```

Within every assignment, every file of a student is compared with every file of every other student. Only files with the given extensions are compared, empty files are skipped. Before comparing, empty lines and comments are removed and whitespace is normalized. The similarity of two files is the share of distinct lines they have in common (Jaccard similarity), from 0% for completely different to 100% for identical files. The similarity of two students in an assignment is the highest similarity of any of their files.

The command lists every pair of students whose similarity reaches the threshold in at least one assignment, with the files that reach it. Similarities of 90% and above are highlighted in red.

| Option | Default | Description |
|---|---|---|
| `-e`, `--extension` | `.html` | File extensions to compare, e.g. `.html,.css,.js` |
| `-t`, `--threshold` | `70` | Similarity in percent from which a pair of students is listed |
| `-i`, `--ignore` | | File names without extension to ignore, e.g. `reset,normalize` |
| `-o`, `--order-by` | `assignment` | Order the results by `assignment` or `student` |
| `-u`, `--student` | | Only list the pairs of this student, e.g. `lastname.firstname` |
| `-n`, `--assignment` | | Only list this assignment |
| `-d`, `--diff` | | Show the diffs of listed cases, see below |
| `-v`, `--verbose` | | Print details of the analysis |

With `--diff`, the command prompts for a case after listing the results: `1` opens the files of all assignments of case 1, `1.2` the files of the second assignment of case 1. Every pair of files is opened side by side in FileMerge (`opendiff`), which is part of the Xcode command line tools on macOS. `p` lists the results again, `q` quits.

```bash
gh mmc check -e .html,.css -t 80 --diff
```

## Codespaces

`gh mmc codespaces list` lists the codespaces owned by the organization, with their state, git status, repository, user, machine, idle timeout, prebuild and last usage. Users that are on the roster are shown by their folder name *lastname*.*firstname*.

`gh mmc codespaces rm` lets you select codespaces and deletes them after confirmation. With `--all`, it selects all codespaces that are not running and have no uncommitted or unpushed changes, so that no work is lost.

Both commands operate on the organization given with `--org`, or else on the organization of the classroom. Outside of a classroom, you will be prompted to select the organization. Run within a course, they only operate on the codespaces of the starter repository and the student repositories of the course, and run within a classroom outside of a course, on the codespaces of all its courses.

Managing the codespaces of an organization requires you to be an owner of the organization and the gh token to have the `admin:org` scope. If it does not have it, the scope is added and removed again afterwards, as for inviting students.

## Cleaning and deleting

`gh mmc clean` removes the local clones of the starter repository and of the student repositories of the course. The repositories on GitHub are not changed, so `gh mmc pull` clones them again. Local clones with uncommitted or unpushed changes are kept, so that no work is lost.

`gh mmc delete` deletes the starter repository and the student repositories of the course on GitHub and locally. This cannot be undone. The command lists all repositories it deletes, including local clones with changes that are not pushed, and asks you to type the name of the classroom to confirm. Deleting the private starter repository on GitHub deletes all its forks as well, e.g. the ones of students that are not on the roster anymore, so the student repositories are deleted first, and the starter repository is kept if any of them cannot be deleted. A local clone is only deleted after its repository on GitHub was deleted. Deleting requires the gh token to have the `delete_repo` scope, which is added for deleting and removed again afterwards if necessary, both requiring you to authenticate in the browser.

Both commands only remove folders that are a clone of the repository, and keep the metadata of the course, so that `gh mmc pull` can set up the course again.

## Status

`gh mmc pull` and `gh mmc sync` list the starter repository first and then every student once, by the local folder and the URL of the repository. The label tells what happened: `Created`, `Updated` (made private or write access granted), `Synced` (`gh mmc sync` only), `Cloned`, `Pulled`, or `Clean` if nothing changed, or `Pending` if the invitation of the student is not accepted yet, `Invalid` or `Failed`. After `gh mmc pull` and `gh mmc sync`, the local clones have the latest state of the repositories on GitHub. A table summarizing the status of the students follows. Statuses no student has are omitted, e.g.:

```
Clean: 2026-1aWI-SWP (https://github.com/HTLD-MMC-TEST/2026-1aWI-SWP-module-ts)
Created: klasen.moritz (https://github.com/HTLD-MMC-TEST/2026-1aWI-SWP-module-ts-MoritzKlasen)
Pending: partoll.sebastian (Scheber08)
Pulled: stauss.schueler (https://github.com/HTLD-MMC-TEST/2026-1aWI-SWP-module-ts-staussh)

Course module-ts in organization HTLD-MMC-TEST:
┌──────────┬──────────┬───────────────────────────────────────┐
│ Status   │ Students │ Meaning                               │
├──────────┼──────────┼───────────────────────────────────────┤
│ Existing │        1 │ repository existed already            │
│ Created  │        1 │ repository created now                │
│ Pending  │        1 │ invitation pending, no repository yet │
├──────────┼──────────┼───────────────────────────────────────┤
│ Total    │        3 │                                       │
└──────────┴──────────┴───────────────────────────────────────┘
```

`gh mmc init` ends with a similar table summarizing the membership of the students on the roster.

See [Commands](#commands) for further details.

## Commands

For more information and a list of available commands

```bash
gh mmc -h
```

## License

This project is licensed under the terms of the MIT open source license. Please refer to [LICENSE](LICENSE) for the full terms.

## Maintainers

See [CODEOWNERS](CODEOWNERS)

## Attribution and Thanks

This extension started out as an extension of the great [GitHub Classroom CLI](https://github.com/github/gh-classroom). It does not use GitHub Classroom anymore, but parts of its code are derived from the GitHub Classroom CLI and are licensed under its original licenses:

- [Original License 1](LICENSE-1.txt)
- [Original License 2](LICENSE-2.txt)