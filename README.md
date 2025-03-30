[![Contributor Covenant](https://img.shields.io/badge/Contributor%20Covenant-2.1-4baaaa.svg)](CODE_OF_CONDUCT.md)

# majikmate Classroom

This extension is an opinionated [GitHub Classroom](https://classroom.github.com) extension for GitHub CLI to easily work with GitHub Classrooms and student repos. Currently, its main purpose is to clone GitHub Classroom assignment and the starter repo as well as to synch back changes from the starter repo to the student repos.

# Installation
- Install the gh cli

  On MacOS, e.g., use [Homebrew](https://brew.sh/)

  ```bash
  brew install gh
  ```

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

See [Commands](#commands) for further details.

### Commands

For more information and a list of available commands

```bash
gh mmc -h
```

## License

This project is licensed under the terms of the MIT open source license. Please refer to [LICENSE](LICENSE) for the full terms.

## Maintainers

See [CODEOWNERS](CODEOWNERS)

## Attribution and Thanks

**GitHub Classroom**

This extension is heavily inspired by the great GitHub Classroom CLI available here:

- [GitHub Classroom CLI](https://github.com/github/gh-classroom)

**Licenses**
- [Orignial License 1](LICENSE-1.txt)
- [Orignial License 2](LICENSE-2.txt)