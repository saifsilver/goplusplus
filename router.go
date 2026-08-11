package gpp

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"sort"
	"strings"

	"github.com/saifsilver/goplusplus/dbcore"
)

// RouterGroup organizes routes under a common path prefix and middleware set.
type RouterGroup struct {
	prefix      string
	middlewares HandlersChain
	engine      *Engine
}

type staticRoute struct {
	mount    string
	handlers HandlersChain
}

// Group creates a new child route group with a path prefix and optional middleware.
func (group *RouterGroup) Group(relativePath string, handlers ...HandlerFunc) *RouterGroup {
	return &RouterGroup{
		prefix:      group.combinePath(relativePath),
		middlewares: group.combineMiddlewares(handlers),
		engine:      group.engine,
	}
}

// RegisterModule mounts a domain module under a relative sub-route group.
func (group *RouterGroup) RegisterModule(relativePath string, module Module) {
	subGroup := group.Group(relativePath)
	module.Register(subGroup)
}

// Use appends middleware to the current route group.
func (group *RouterGroup) Use(middlewares ...HandlerFunc) {
	group.middlewares = append(group.middlewares, middlewares...)
}

// Handle registers a new request handler for a given HTTP method and path.
func (group *RouterGroup) Handle(httpMethod, relativePath string, handlers ...HandlerFunc) {
	absolutePath := group.combinePath(relativePath)
	finalHandlers := group.combineMiddlewares(handlers)
	group.engine.addRoute(httpMethod, absolutePath, finalHandlers)
}

// GET registers a GET route.
func (group *RouterGroup) GET(relativePath string, handlers ...HandlerFunc) {
	group.Handle(http.MethodGet, relativePath, handlers...)
}

// POST registers a POST route.
func (group *RouterGroup) POST(relativePath string, handlers ...HandlerFunc) {
	group.Handle(http.MethodPost, relativePath, handlers...)
}

// PUT registers a PUT route.
func (group *RouterGroup) PUT(relativePath string, handlers ...HandlerFunc) {
	group.Handle(http.MethodPut, relativePath, handlers...)
}

// DELETE registers a DELETE route.
func (group *RouterGroup) DELETE(relativePath string, handlers ...HandlerFunc) {
	group.Handle(http.MethodDelete, relativePath, handlers...)
}

// PATCH registers a PATCH route.
func (group *RouterGroup) PATCH(relativePath string, handlers ...HandlerFunc) {
	group.Handle(http.MethodPatch, relativePath, handlers...)
}

// OPTIONS registers an OPTIONS route.
func (group *RouterGroup) OPTIONS(relativePath string, handlers ...HandlerFunc) {
	group.Handle(http.MethodOptions, relativePath, handlers...)
}

// HEAD registers a HEAD route.
func (group *RouterGroup) HEAD(relativePath string, handlers ...HandlerFunc) {
	group.Handle(http.MethodHead, relativePath, handlers...)
}

// Any registers a route for all common HTTP methods.
func (group *RouterGroup) Any(relativePath string, handlers ...HandlerFunc) {
	methods := []string{
		http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete,
		http.MethodPatch, http.MethodOptions, http.MethodHead,
	}
	for _, method := range methods {
		group.Handle(method, relativePath, handlers...)
	}
}

// Static serves static files from a root directory under a URL path prefix.
func (group *RouterGroup) Static(relativePath, root string) {
	handler := func(c *Context) error {
		fileServer := http.StripPrefix(group.combinePath(relativePath), http.FileServer(http.Dir(root)))
		fileServer.ServeHTTP(c.Writer, c.Request)
		return nil
	}
	urlPattern := path.Join(relativePath, "*filepath")
	group.GET(urlPattern, handler)
	group.HEAD(urlPattern, handler)
}

// StaticFS serves embedded static files from an io/fs.FS filesystem (e.g. embed.FS) with SPA fallback routing to index.html.
func (group *RouterGroup) StaticFS(relativePath string, fsys fs.FS) {
	if fsys == nil {
		panic("gpp: static filesystem cannot be nil")
	}
	mount := normalizeStaticMount(group.combinePath(relativePath))
	fileServer := http.FileServer(http.FS(fsys))
	handler := func(c *Context) error {
		relative, ok := staticRelativePath(mount, c.Request.URL.Path)
		if !ok {
			return ErrNotFound("Static resource not found")
		}
		servePath, exists := resolveStaticPath(fsys, relative)
		if !exists {
			if mount != "/" {
				return ErrNotFound("Static resource not found")
			}
			servePath, exists = resolveStaticPath(fsys, "index.html")
			if !exists {
				return ErrNotFound("Static resource not found")
			}
		}
		request := c.Request.Clone(c.Request.Context())
		urlCopy := *c.Request.URL
		urlCopy.Path = staticServerPath(servePath)
		urlCopy.RawPath = ""
		request.URL = &urlCopy
		fileServer.ServeHTTP(c.Writer, request)
		return nil
	}
	group.engine.addStaticRoute(mount, group.combineMiddlewares(HandlersChain{handler}))
}

func (engine *Engine) addStaticRoute(mount string, handlers HandlersChain) {
	engine.staticMu.Lock()
	defer engine.staticMu.Unlock()
	engine.staticRoutes = append(engine.staticRoutes, staticRoute{mount: mount, handlers: handlers})
	sort.SliceStable(engine.staticRoutes, func(i, j int) bool {
		return len(engine.staticRoutes[i].mount) > len(engine.staticRoutes[j].mount)
	})
}

func (engine *Engine) matchStaticRoute(method, requestPath string) HandlersChain {
	if method != http.MethodGet && method != http.MethodHead {
		return nil
	}
	engine.staticMu.RLock()
	defer engine.staticMu.RUnlock()
	for _, route := range engine.staticRoutes {
		if _, ok := staticRelativePath(route.mount, requestPath); ok {
			return route.handlers
		}
	}
	return nil
}

func normalizeStaticMount(mount string) string {
	if mount == "" || mount == "." {
		return "/"
	}
	mount = path.Clean("/" + strings.TrimPrefix(mount, "/"))
	if mount != "/" {
		mount = strings.TrimSuffix(mount, "/")
	}
	return mount
}

func staticRelativePath(mount, requestPath string) (string, bool) {
	if mount != "/" && requestPath != mount && !strings.HasPrefix(requestPath, mount+"/") {
		return "", false
	}
	relative := requestPath
	if mount != "/" {
		relative = strings.TrimPrefix(requestPath, mount)
	}
	relative = strings.TrimPrefix(relative, "/")
	for _, segment := range strings.Split(relative, "/") {
		if segment == "." || segment == ".." {
			return "", false
		}
	}
	cleaned := strings.TrimPrefix(path.Clean("/"+relative), "/")
	if cleaned == "." || cleaned == "" {
		return "index.html", true
	}
	if !fs.ValidPath(cleaned) {
		return "", false
	}
	return cleaned, true
}

func resolveStaticPath(fsys fs.FS, relative string) (string, bool) {
	file, err := fsys.Open(relative)
	if err != nil {
		return "", false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", false
	}
	if info.IsDir() {
		index := path.Join(relative, "index.html")
		if indexFile, err := fsys.Open(index); err == nil {
			_ = indexFile.Close()
			return index, true
		}
	}
	return relative, true
}

func staticServerPath(relative string) string {
	if path.Base(relative) == "index.html" {
		directory := strings.TrimSuffix(relative, "index.html")
		return "/" + directory
	}
	return "/" + relative
}

// StaticEmbed mounts an embedded filesystem (embed.FS) with zero-boilerplate automatic subfolder resolution (dist, build, static, public) and SPA index.html fallback.
func (group *RouterGroup) StaticEmbed(relativePath string, embedFS embed.FS, optionalSubDir ...string) {
	var targetFS fs.FS = embedFS

	if len(optionalSubDir) > 0 && optionalSubDir[0] != "" {
		if sub, err := fs.Sub(embedFS, optionalSubDir[0]); err == nil {
			targetFS = sub
		}
	} else {
		// Auto-detect common frontend build output folders
		for _, candidates := range []string{"dist", "build", "static", "public", "web/dist", "web/build"} {
			if sub, err := fs.Sub(embedFS, candidates); err == nil {
				if _, errCheck := sub.Open("index.html"); errCheck == nil {
					targetFS = sub
					break
				}
			}
		}
	}

	group.StaticFS(relativePath, targetFS)
}

func (group *RouterGroup) combinePath(relativePath string) string {
	if relativePath == "" {
		return group.prefix
	}
	finalPath := path.Join(group.prefix, relativePath)
	if strings.HasSuffix(relativePath, "/") && !strings.HasSuffix(finalPath, "/") {
		return finalPath + "/"
	}
	return finalPath
}

func (group *RouterGroup) combineMiddlewares(handlers HandlersChain) HandlersChain {
	merged := make(HandlersChain, 0, len(group.middlewares)+len(handlers))
	merged = append(merged, group.middlewares...)
	merged = append(merged, handlers...)
	return merged
}

// BindResource automatically mounts a complete set of RESTful CRUD routes (GET /, GET /:id, POST /, PUT /:id, DELETE /:id) for entity T using dbcore.Repository[T].
func BindResource[T any](group *RouterGroup, relativePath string, repo *dbcore.Repository[T]) {
	sub := group.Group(relativePath)

	sub.GET("", func(c *Context) error {
		page, limit := c.GetPageAndLimit(20)
		items, total, err := repo.Paginate(c.Request.Context(), page, limit)
		if err != nil {
			return NewInternalError("resource.list", err, WithErrorCategory("database"))
		}
		return c.Paginate(http.StatusOK, items, page, limit, total)
	})

	sub.GET("/:id", func(c *Context) error {
		id := c.Param("id")
		item, err := repo.FindByID(c.Request.Context(), id)
		if err != nil {
			return c.NotFound("Resource not found")
		}
		return c.OK(item)
	})

	sub.POST("", func(c *Context) error {
		var entity T
		if err := c.BindAndValidate(&entity); err != nil {
			return err
		}
		if err := repo.Create(c.Request.Context(), &entity); err != nil {
			return NewInternalError("resource.create", err, WithErrorCategory("database"))
		}
		return c.Created(entity)
	})

	sub.PUT("/:id", func(c *Context) error {
		id := c.Param("id")
		var entity T
		if err := c.BindAndValidate(&entity); err != nil {
			return err
		}
		if err := repo.Update(c.Request.Context(), id, &entity); err != nil {
			return NewInternalError("resource.update", err, WithErrorCategory("database"))
		}
		return c.OK(entity)
	})

	sub.DELETE("/:id", func(c *Context) error {
		id := c.Param("id")
		if err := repo.Delete(c.Request.Context(), id); err != nil {
			return c.NotFound("Resource not found")
		}
		return c.NoContent()
	})
}
