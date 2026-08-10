package gpp

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// RouterGroup organizes routes under a common path prefix and middleware set.
type RouterGroup struct {
	prefix      string
	middlewares HandlersChain
	engine      *Engine
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
	fileServer := http.FileServer(http.FS(fsys))
	handler := func(c *Context) error {
		reqPath := strings.TrimPrefix(c.Request.URL.Path, group.combinePath(relativePath))
		if reqPath == "" || reqPath == "/" {
			reqPath = "index.html"
		}
		f, err := fsys.Open(strings.TrimPrefix(reqPath, "/"))
		if err != nil {
			// SPA fallback routing to index.html for client-side React/Vite/Vue Router
			c.Request.URL.Path = path.Join(group.combinePath(relativePath), "index.html")
		} else {
			_ = f.Close()
		}
		fileServer.ServeHTTP(c.Writer, c.Request)
		return nil
	}
	urlPattern := path.Join(relativePath, "*filepath")
	group.GET(urlPattern, handler)
	group.HEAD(urlPattern, handler)
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
