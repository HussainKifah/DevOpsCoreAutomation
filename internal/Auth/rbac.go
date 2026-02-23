package auth

import (
	"fmt"

	"gorm.io/gorm"
)

// system permission
type Permission struct {
	gorm.Model
	Name        string `json:"name" gorm:"uniqueIndex;not null;size:50"`
	Resource    string `json:"resource" gorm:"not null;size:50"`
	Action      string `json:"action" gorm:"not null;size:20"`
	Description string `json:"description" gorm:"size:200"`
}

// UserRole
type Role struct {
	gorm.Model
	Name        string       `json:"name" gorm:"uniqueIndex;not null;size:50"`
	Description string       `json:"description" gorm:"size:200"`
	Permissions []Permission `json:"permissions" gorm:"many2many:role_permissions;"`
}

// UserRole links users to roles
type UserRole struct {
	UserID uint `json:"user_id" gorm:"primaryKey"`
	RoleID uint `json:"role_id" gorm:"primaryKey"`
}

// RABC manages role-based access control
type RBAC struct {
	permissions map[string]Permission
	roles       map[string]Role
	userRoles   map[uint][]Role
}

func NewRBAC() *RBAC {
	return &RBAC{
		permissions: make(map[string]Permission),
		roles:       make(map[string]Role),
		userRoles:   make(map[uint][]Role),
	}
}

func (r *RBAC) AddPermission(name, resource, action, description string) {
	permission := Permission{
		Name:        name,
		Resource:    resource,
		Action:      action,
		Description: description,
	}
	r.permissions[name] = permission
}

func (r *RBAC) AddRole(name, description string, permissionNames []string) {
	role := Role{
		Name:        name,
		Description: description,
		Permissions: make([]Permission, 0),
	}

	//Add permissions to role
	for _, perName := range permissionNames {
		if perm, exists := r.permissions[perName]; exists {
			role.Permissions = append(role.Permissions, perm)
		}
	}
	r.roles[name] = role
}

func (r *RBAC) AssignRole(userID uint, roleName string) {
	if role, exists := r.roles[roleName]; exists {
		if _, userExists := r.userRoles[userID]; !userExists {
			r.userRoles[userID] = make([]Role, 0)
		}
		r.userRoles[userID] = append(r.userRoles[userID], role)
	}
}

func (r *RBAC) HasPermission(userID uint, resource, action string) bool {
	userRoles, exists := r.userRoles[userID]
	if !exists {
		return false
	}

	for _, role := range userRoles {
		for _, permission := range role.Permissions {
			if permission.Resource == resource && permission.Action == action {
				return true
			}
		}
	}
	return false
}

func (r *RBAC) HasRole(userID uint, roleName string) bool {
	userRoles, exists := r.userRoles[userID]
	if !exists {
		return false
	}
	for _, role := range userRoles {
		if role.Name == roleName {
			return true
		}
	}
	return false
}

func (r *RBAC) GetUserPermissions(userID uint) []string {
	var permissions []string
	permissionSet := make(map[string]bool)

	userRoles, exists := r.userRoles[userID]
	if !exists {
		return permissions
	}

	for _, role := range userRoles {
		for _, permission := range role.Permissions {
			permkey := fmt.Sprintf("%s:%s", permission.Resource, permission.Action)
			if !permissionSet[permkey] {
				permissions = append(permissions, permkey)
				permissionSet[permkey] = true
			}
		}
	}
	return permissions
}

// func (r *RBAC) InitializeDefaultRoles() {}
