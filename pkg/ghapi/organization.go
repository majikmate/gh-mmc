package ghapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
)

type GitHubOrganizationInvitation struct {
	Login string `json:"login"`
	Email string `json:"email"`
}

// MembershipStatus is the status of a user in an organization
type MembershipStatus string

const (
	// MembershipNone means the user is neither a member of the organization nor invited to it
	MembershipNone MembershipStatus = "not a member"
	// MembershipActive means the user is a member of the organization
	MembershipActive MembershipStatus = "member"
	// MembershipPending means the user was invited to the organization before and has not accepted yet
	MembershipPending MembershipStatus = "invitation pending"
	// MembershipInvited means the user was invited to the organization now
	MembershipInvited MembershipStatus = "invited"
)

// ListOrganizationMembers returns all members of an organization, including its owners
func ListOrganizationMembers(client *api.RESTClient, orgName string) ([]GitHubUser, error) {
	return getAllPages[GitHubUser](client, fmt.Sprintf("orgs/%s/members", orgName))
}

// ListOrganizationOwners returns all owners of an organization
func ListOrganizationOwners(client *api.RESTClient, orgName string) ([]GitHubUser, error) {
	return getAllPages[GitHubUser](client, fmt.Sprintf("orgs/%s/members?role=admin", orgName))
}

// ListOrganizationInvitations returns all pending invitations of an organization
func ListOrganizationInvitations(client *api.RESTClient, orgName string) ([]GitHubOrganizationInvitation, error) {
	return getAllPages[GitHubOrganizationInvitation](client, fmt.Sprintf("orgs/%s/invitations", orgName))
}

// GetOrganizationRole returns the role of the logged in user in the organization, i.e. admin for owners or member
func GetOrganizationRole(client *api.RESTClient, orgName string) (string, error) {
	var response struct {
		Role string `json:"role"`
	}

	err := client.Get(fmt.Sprintf("user/memberships/orgs/%s", orgName), &response)
	if err != nil {
		return "", err
	}

	return response.Role, nil
}

// GetUser returns the user with the given login
func GetUser(client *api.RESTClient, login string) (GitHubUser, error) {
	var response GitHubUser

	err := client.Get(fmt.Sprintf("users/%s", login), &response)
	if err != nil {
		return GitHubUser{}, err
	}

	return response, nil
}

// InviteToOrganization invites the user to become a direct member of the organization
func InviteToOrganization(client *api.RESTClient, orgName string, userId int) error {
	body, err := json.Marshal(map[string]any{
		"invitee_id": userId,
		"role":       "direct_member",
	})
	if err != nil {
		return err
	}

	return client.Post(fmt.Sprintf("orgs/%s/invitations", orgName), bytes.NewReader(body), nil)
}

// Invitee is a user to be invited to an organization
type Invitee struct {
	Login string
	Email string
}

// OrganizationMembership holds the members and the pending invitations of an organization
type OrganizationMembership struct {
	org         string
	members     map[string]bool
	invitations map[string]bool
	invited     map[string]bool
}

// LoadOrganizationMembership loads the members and the pending invitations of an organization
func LoadOrganizationMembership(client *api.RESTClient, orgName string) (*OrganizationMembership, error) {
	members, err := ListOrganizationMembers(client, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to list members of organization %s: %v", orgName, err)
	}

	invitations, err := ListOrganizationInvitations(client, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to list pending invitations of organization %s: %v", orgName, err)
	}

	m := &OrganizationMembership{
		org:         orgName,
		members:     make(map[string]bool),
		invitations: make(map[string]bool),
		invited:     make(map[string]bool),
	}
	for _, member := range members {
		m.members[strings.ToLower(member.Login)] = true
	}
	// Invitations by email have no login
	for _, invitation := range invitations {
		if invitation.Login != "" {
			m.invitations[strings.ToLower(invitation.Login)] = true
		}
		if invitation.Email != "" {
			m.invitations[strings.ToLower(invitation.Email)] = true
		}
	}

	return m, nil
}

// Status returns the membership status of the user with the login and email. A pending invitation is found by
// login or by email.
func (m *OrganizationMembership) Status(login string, email string) MembershipStatus {
	switch {
	case m.members[strings.ToLower(login)]:
		return MembershipActive
	case m.invited[strings.ToLower(login)]:
		return MembershipInvited
	case m.invitations[strings.ToLower(login)], email != "" && m.invitations[strings.ToLower(email)]:
		return MembershipPending
	}
	return MembershipNone
}

// InviteMissing invites all users that are neither members of the organization nor invited to it already.
// Inviting requires the logged in user to be an owner of the organization and the gh token to have the admin:org
// scope. If it does not have it, the scope is added for inviting and removed again afterwards. It returns the
// client to use afterwards and the errors of the users that could not be invited by lowercase login.
func (m *OrganizationMembership) InviteMissing(client *api.RESTClient, users []Invitee) (*api.RESTClient, map[string]error) {
	errs := make(map[string]error)

	missing := []string{}
	for _, u := range users {
		login := strings.ToLower(u.Login)
		switch {
		case login == "":
			errs[login] = errors.New("no GitHub user")
		case m.Status(u.Login, u.Email) == MembershipNone && !slices.Contains(missing, login):
			missing = append(missing, login)
		}
	}

	// Look up the users first, so that the admin:org scope is only requested if a user is actually invited
	invitees := []GitHubUser{}
	for _, login := range missing {
		user, err := GetUser(client, login)
		if err != nil {
			errs[login] = fmt.Errorf("failed to get GitHub user %s: %v", login, err)
			continue
		}
		invitees = append(invitees, user)
	}
	if len(invitees) == 0 {
		return client, errs
	}

	fail := func(err error) {
		for _, user := range invitees {
			errs[strings.ToLower(user.Login)] = err
		}
	}

	role, err := GetOrganizationRole(client, m.org)
	if err != nil {
		fail(fmt.Errorf("failed to get the role in organization %s: %v", m.org, err))
		return client, errs
	}
	if role != "admin" {
		fail(fmt.Errorf("only owners of organization %s can invite members", m.org))
		return client, errs
	}

	client, err = WithScope(client, "admin:org", func(client *api.RESTClient) error {
		for _, user := range invitees {
			err := InviteToOrganization(client, m.org, user.Id)
			if err != nil {
				errs[strings.ToLower(user.Login)] = fmt.Errorf("failed to invite %s to organization %s: %v", user.Login, m.org, err)
				continue
			}
			m.invited[strings.ToLower(user.Login)] = true
		}
		return nil
	})
	if err != nil {
		fail(err)
	}

	return client, errs
}
