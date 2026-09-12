package cloud

import (
	"errors"
	"fmt"
)

// CheckTeamMembership checks current control-plane membership for an existing
// session. Tenant user rows and cached stores do not prove an invitation is active.
func CheckTeamMembership(store Store, memberAccountID, ownerAccountID string) error {
	member, err := store.GetAccountByID(memberAccountID)
	if err != nil {
		return fmt.Errorf("read team account: %w", err)
	}
	if member == nil || !member.EmailVerified || member.Email == "" {
		return errors.New("team account unavailable")
	}
	invite, err := store.GetAcceptedInviteByEmail(member.Email)
	if err != nil {
		return fmt.Errorf("read team membership: %w", err)
	}
	if invite == nil || !invite.Accepted || invite.AccountID != ownerAccountID {
		return errors.New("team membership is no longer active")
	}
	return nil
}
