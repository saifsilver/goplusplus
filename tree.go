package gpp

import (
	"net/url"
	"strings"
)

// Param represents a single URL path parameter (e.g., key="id", value="123").
type Param struct {
	Key   string
	Value string
}

// Params is a slice of Param values matching the request route.
type Params []Param

// Get returns the value of the first matching parameter key, or empty string if not found.
func (ps Params) Get(key string) string {
	for i := range ps {
		if ps[i].Key == key {
			return ps[i].Value
		}
	}
	return ""
}

// nodeType represents the classification of a Radix Tree node.
type nodeType uint8

const (
	nodeStatic nodeType = iota
	nodeParam
	nodeCatchAll
)

type node struct {
	path      string
	indices   string
	wildChild bool
	nType     nodeType
	paramName string
	children  []*node
	handlers  HandlersChain
}

func (n *node) addRoute(path string, handlers HandlersChain) {
	fullPath := path
	n.insertChild(fullPath, path, handlers)
}

func (n *node) insertChild(fullPath, path string, handlers HandlersChain) {
	for {
		// Find longest common prefix
		i := longestCommonPrefix(path, n.path)

		// Split node if prefix is smaller than n.path
		if i < len(n.path) {
			child := node{
				path:      n.path[i:],
				indices:   n.indices,
				wildChild: n.wildChild,
				nType:     n.nType,
				paramName: n.paramName,
				children:  n.children,
				handlers:  n.handlers,
			}

			n.children = []*node{&child}
			n.indices = string([]byte{n.path[i]})
			n.path = path[:i]
			n.handlers = nil
			n.wildChild = false
			n.paramName = ""
			n.nType = nodeStatic
		}

		// Make new node a child of this node
		if i < len(path) {
			path = path[i:]

			if n.wildChild {
				n = n.children[0]
				continue
			}

			c := path[0]

			// Param or wildcard node
			if (n.nType == nodeParam || n.nType == nodeCatchAll) && c == '/' && len(n.children) == 1 {
				n = n.children[0]
				continue
			}

			// Check if child with first byte exists
			if idx := strings.IndexByte(n.indices, c); idx != -1 {
				n = n.children[idx]
				continue
			}

			// Add child
			if c == ':' || c == '*' {
				var childType nodeType = nodeParam
				if c == '*' {
					childType = nodeCatchAll
				}

				// Find end of parameter
				end := strings.IndexByte(path, '/')
				var paramSegment string
				var pName string
				if end == -1 {
					paramSegment = path
					pName = path[1:]
				} else {
					paramSegment = path[:end]
					pName = path[1:end]
				}

				child := &node{
					path:      paramSegment,
					nType:     childType,
					paramName: pName,
					handlers:  handlers,
				}

				n.wildChild = true
				n.children = append(n.children, child)

				if end != -1 {
					child.handlers = nil
					subChild := &node{}
					child.children = []*node{subChild}
					subChild.insertChild(fullPath, path[end:], handlers)
				}
				return
			}

			// Regular static child node
			child := &node{
				path:     path,
				handlers: handlers,
			}
			n.indices += string([]byte{c})
			n.children = append(n.children, child)
			return
		}

		// Exact path match reached
		n.handlers = handlers
		return
	}
}

func (n *node) getValue(path string, params *Params) (handlers HandlersChain) {
walk:
	for {
		prefix := n.path
		if len(path) > len(prefix) {
			if path[:len(prefix)] == prefix {
				path = path[len(prefix):]

				if !n.wildChild {
					c := path[0]
					if idx := strings.IndexByte(n.indices, c); idx != -1 {
						n = n.children[idx]
						continue walk
					}
					return nil
				}

				// Handle parameter or wildcard children
				n = n.children[0]
				switch n.nType {
				case nodeParam:
					end := strings.IndexByte(path, '/')
					if end == -1 {
						end = len(path)
					}
					val := path[:end]
					if strings.IndexByte(val, '%') != -1 {
						if unescaped, err := url.PathUnescape(val); err == nil {
							val = unescaped
						}
					}
					if params != nil && n.paramName != "" {
						*params = append(*params, Param{Key: n.paramName, Value: val})
					}
					if end < len(path) {
						if len(n.children) > 0 {
							path = path[end:]
							n = n.children[0]
							continue walk
						}
						return nil
					}
					return n.handlers
				case nodeCatchAll:
					val := path
					if strings.IndexByte(val, '%') != -1 {
						if unescaped, err := url.PathUnescape(val); err == nil {
							val = unescaped
						}
					}
					if params != nil && n.paramName != "" {
						*params = append(*params, Param{Key: n.paramName, Value: val})
					}
					return n.handlers
				}
			}
		} else if path == prefix {
			return n.handlers
		}
		return nil
	}
}

func longestCommonPrefix(a, b string) int {
	i := 0
	max := len(a)
	if len(b) < max {
		max = len(b)
	}
	for i < max && a[i] == b[i] {
		i++
	}
	return i
}
