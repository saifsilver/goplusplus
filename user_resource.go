package gpp

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/saifsilver/goplusplus/dbcore"
)

// BindUserResource automatically mounts complete, user-scoped RESTful CRUD routes
// (GET /, GET /:id, POST /, PUT /:id, DELETE /:id) for entity T.
// All operations are strictly filtered and authorized by the logged-in User's ID.
func BindUserResource[T any](target any, relativePath string, tableName ...string) {
	var group *RouterGroup
	var client *dbcore.Client

	switch t := target.(type) {
	case *Engine:
		group = &t.RouterGroup
		client = t.dbClient
	case *RouterGroup:
		group = t
		if t.engine != nil {
			client = t.engine.dbClient
		}
	default:
		panic("gpp: BindUserResource target must be *Engine or *RouterGroup")
	}

	if client == nil {
		panic("gpp: BindUserResource requires an initialized database connection")
	}

	sub := group.Group(relativePath)
	orm := dbcore.NewORM[T](client, tableName...)



	// GET list (user-scoped with pagination)
	sub.GET("", func(c *Context) error {
		userID, err := c.RequireUserID()
		if err != nil {
			return err
		}
		page, limit := c.GetPageAndLimit(20)
		items, total, err := orm.Where("user_id", userID).Paginate(c.Request.Context(), page, limit)
		if err != nil {
			return NewInternalError("resource.list", err, WithErrorCategory("database"))
		}
		return c.Paginate(http.StatusOK, items, page, limit, total)
	})

	// GET by ID (user-scoped)
	sub.GET("/:id", func(c *Context) error {
		userID, err := c.RequireUserID()
		if err != nil {
			return err
		}
		id := c.Param("id")
		item, err := orm.FindByID(c.Request.Context(), id)
		if err != nil {
			return c.NotFound("Resource not found")
		}
		if ownerID, ok := getUserIDFromStruct(reflect.ValueOf(item)); ok && ownerID != userID {
			return c.NotFound("Resource not found")
		}
		return c.OK(item)
	})

	// POST create (auto-assigns userID)
	sub.POST("", func(c *Context) error {
		userID, err := c.RequireUserID()
		if err != nil {
			return err
		}
		var entity T
		if err := c.BindAndValidate(&entity); err != nil {
			return err
		}
		setUserIDOnStruct(reflect.ValueOf(&entity), userID)

		if err := orm.Save(c.Request.Context(), &entity); err != nil {
			return NewInternalError("resource.create", err, WithErrorCategory("database"))
		}
		return c.Created(entity)
	})

	// PUT update (user-scoped ownership verification)
	sub.PUT("/:id", func(c *Context) error {
		userID, err := c.RequireUserID()
		if err != nil {
			return err
		}
		id := c.Param("id")
		existing, err := orm.FindByID(c.Request.Context(), id)
		if err != nil {
			return c.NotFound("Resource not found")
		}
		if ownerID, ok := getUserIDFromStruct(reflect.ValueOf(existing)); ok && ownerID != userID {
			return c.NotFound("Resource not found")
		}

		var entity T
		if err := c.BindAndValidate(&entity); err != nil {
			return err
		}
		setUserIDOnStruct(reflect.ValueOf(&entity), userID)
		setIDOnStruct(reflect.ValueOf(&entity), id)

		if err := orm.Save(c.Request.Context(), &entity); err != nil {
			return NewInternalError("resource.update", err, WithErrorCategory("database"))
		}
		return c.OK(entity)
	})

	// DELETE (user-scoped ownership verification)
	sub.DELETE("/:id", func(c *Context) error {
		userID, err := c.RequireUserID()
		if err != nil {
			return err
		}
		id := c.Param("id")
		existing, err := orm.FindByID(c.Request.Context(), id)
		if err != nil {
			return c.NotFound("Resource not found")
		}
		if ownerID, ok := getUserIDFromStruct(reflect.ValueOf(existing)); ok && ownerID != userID {
			return c.NotFound("Resource not found")
		}

		if err := orm.Delete(c.Request.Context(), existing); err != nil {

			return NewInternalError("resource.delete", err, WithErrorCategory("database"))
		}
		return c.OK(H{"status": "deleted", "id": id})
	})
}

func getUserIDFromStruct(v reflect.Value) (int64, bool) {

	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return 0, false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return 0, false
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		dbTag := field.Tag.Get("db")
		if field.Name == "UserID" || strings.Contains(dbTag, "user_id") || strings.Contains(dbTag, "user_scope") {
			val := v.Field(i)
			if val.Kind() == reflect.Int64 || val.Kind() == reflect.Int {
				return val.Int(), true
			}
		}
	}
	return 0, false
}

func setUserIDOnStruct(v reflect.Value, userID int64) bool {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return false
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		dbTag := field.Tag.Get("db")
		if field.Name == "UserID" || strings.Contains(dbTag, "user_id") || strings.Contains(dbTag, "user_scope") {
			val := v.Field(i)
			if val.CanSet() {
				if val.Kind() == reflect.Int64 || val.Kind() == reflect.Int {
					val.SetInt(userID)
					return true
				}
			}
		}
	}
	return false
}

func setIDOnStruct(v reflect.Value, idStr string) bool {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return false
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		dbTag := field.Tag.Get("db")
		if field.Name == "ID" || strings.Contains(dbTag, "pk") {
			val := v.Field(i)
			if val.CanSet() {
				if val.Kind() == reflect.Int64 || val.Kind() == reflect.Int {
					var id int64
					fmt.Sscanf(idStr, "%d", &id)
					val.SetInt(id)
					return true
				} else if val.Kind() == reflect.String {
					val.SetString(idStr)
					return true
				}
			}
		}
	}
	return false
}
