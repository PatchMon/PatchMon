package store

import (
	"context"
	"errors"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
)

var (
	// ErrSetupCompleted means first-run setup has already created a user on this install.
	ErrSetupCompleted = errors.New("first-run setup already completed")
	// ErrLastSuperadmin means the change would leave no active superadmin.
	ErrLastSuperadmin = errors.New("change would leave no active superadmin")
	// ErrLastAdmin means the change would leave no active admin or superadmin.
	ErrLastAdmin = errors.New("change would leave no active admin")
)

// SetupCompleted reports whether first-run setup has finished: the install
// latch is set, or any user exists.
func (s *UsersStore) SetupCompleted(ctx context.Context) (bool, error) {
	done, err := s.db.DB(ctx).Queries.SetupCompleted(ctx)
	if err != nil {
		return false, err
	}
	return done != nil && *done, nil
}

// CreateFirstAdmin creates the install's first user and sets the setup latch
// in one transaction, returning ErrSetupCompleted if setup has already run.
func (s *UsersStore) CreateFirstAdmin(ctx context.Context, u *models.User, audit AuditEntry) error {
	return s.createFirst(ctx, u, audit, func(q *db.Queries) error {
		return q.CreateUser(ctx, userParams(u))
	})
}

// CreateFirstOidcUser is CreateFirstAdmin for a user created on first SSO login.
func (s *UsersStore) CreateFirstOidcUser(ctx context.Context, u *models.User, oidcSub, oidcProvider string, avatarURL *string, audit AuditEntry) error {
	return s.createFirst(ctx, u, audit, func(q *db.Queries) error {
		return q.CreateOidcUser(ctx, oidcUserParams(u, oidcSub, oidcProvider, avatarURL))
	})
}

func (s *UsersStore) createFirst(ctx context.Context, u *models.User, audit AuditEntry, insert func(*db.Queries) error) error {
	return s.inAdminTx(ctx, func(q *db.Queries) error {
		done, err := q.SetupCompleted(ctx)
		if err != nil {
			return err
		}
		if done != nil && *done {
			return ErrSetupCompleted
		}
		if err := insert(q); err != nil {
			return err
		}
		n, err := q.MarkSetupCompleted(ctx)
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrSetupCompleted
		}
		audit.TargetUserID = u.ID
		if audit.ActorUserID == "" {
			audit.ActorUserID = u.ID
		}
		return insertAuditLog(ctx, q, audit)
	})
}

// UpdateKeepingAdmin applies Update, refusing with ErrLastSuperadmin or
// ErrLastAdmin if it would leave the install without an active administrator.
func (s *UsersStore) UpdateKeepingAdmin(ctx context.Context, u *models.User) error {
	return s.inAdminTx(ctx, func(q *db.Queries) error {
		before, err := q.GetUserByID(ctx, u.ID)
		if err != nil {
			return err
		}
		if err := q.UpdateUser(ctx, updateUserParams(u)); err != nil {
			return err
		}
		wasSuper := before.Role == "superadmin" && before.IsActive
		if wasSuper && (u.Role != "superadmin" || !u.IsActive) {
			n, err := q.CountSuperadmins(ctx)
			if err != nil {
				return err
			}
			if n == 0 {
				return ErrLastSuperadmin
			}
		}
		n, err := q.CountAdmins(ctx)
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrLastAdmin
		}
		return nil
	})
}

// DeleteKeepingAdmin deletes a user, refusing with ErrLastSuperadmin or
// ErrLastAdmin under the same lock as UpdateKeepingAdmin.
func (s *UsersStore) DeleteKeepingAdmin(ctx context.Context, id string) error {
	return s.inAdminTx(ctx, func(q *db.Queries) error {
		target, err := q.GetUserByID(ctx, id)
		if err != nil {
			return err
		}
		switch target.Role {
		case "superadmin":
			n, err := q.CountSuperadmins(ctx)
			if err != nil {
				return err
			}
			if n <= 1 {
				return ErrLastSuperadmin
			}
		case "admin":
			supers, err := q.CountSuperadmins(ctx)
			if err != nil {
				return err
			}
			if supers == 0 {
				admins, err := q.CountActiveAdmins(ctx)
				if err != nil {
					return err
				}
				if admins <= 1 {
					return ErrLastAdmin
				}
			}
		}
		return q.DeleteUser(ctx, id)
	})
}

func (s *UsersStore) inAdminTx(ctx context.Context, fn func(*db.Queries) error) error {
	d := s.db.DB(ctx)
	tx, err := d.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := d.Queries.WithTx(tx)
	if err := q.LockAdminSet(ctx); err != nil {
		return err
	}
	if err := fn(q); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
