package treemux

import (
	"fmt"
	"regexp"
	"strings"
)

type node[T HandlerConstraint] struct {
	path     string
	fullPath string

	priority int

	// The list of static children to check.
	staticIndices []byte
	staticChild   []*node[T]

	// If static routes don't match, check the wildcard children.
	wildcardChild *node[T]

	// If none of the above match, check regular expression routes.
	regexChild []*node[T]
	regExpr    *regexp.Regexp

	// If none of the above match, then we use the catch-all, if applicable.
	catchAllChild *node[T]

	// Data for the node is below.

	addSlash   bool
	isCatchAll bool
	isRegex    bool
	// If true, the head handler was set implicitly, so let it also be set explicitly.
	implicitHead bool
	// If this node is the end of the URL, then call the handler, if applicable.
	leafHandlers map[string]T

	// The names of the parameters to apply.
	leafParamNames []string
}

func (n *node[_]) sortStaticChild(i int) {
	for i > 0 && n.staticChild[i].priority > n.staticChild[i-1].priority {
		n.staticChild[i], n.staticChild[i-1] = n.staticChild[i-1], n.staticChild[i]
		n.staticIndices[i], n.staticIndices[i-1] = n.staticIndices[i-1], n.staticIndices[i]
		i -= 1
	}
}

func (n *node[T]) setHandler(verb string, handler T, implicitHead bool) {
	if n.leafHandlers == nil {
		n.leafHandlers = make(map[string]T)
	}
	_, ok := n.leafHandlers[verb]
	if ok && (verb != "HEAD" || !n.implicitHead) {
		panic(fmt.Sprintf("treemux: %s already handles %s", n.path, verb))
	}
	n.leafHandlers[verb] = handler

	if verb == "HEAD" {
		n.implicitHead = implicitHead
	}
}

func (n *node[T]) addPath(path string, paramNames []string, inStaticToken bool) *node[T] {
	leaf := len(path) == 0
	if leaf {
		if paramNames != nil {
			// Make sure the current param names are the same as the old ones.
			// If not then we have an ambiguous path.
			if n.leafParamNames != nil {
				if len(n.leafParamNames) != len(paramNames) {
					// This should never happen.
					panic("treemux: Reached leaf node with differing wildcard array length. Please report this as a bug.")
				}

				for i := 0; i < len(paramNames); i++ {
					if n.leafParamNames[i] != paramNames[i] {
						panic(fmt.Sprintf("treemux: wildcards %v are ambiguous with wildcards %v",
							n.leafParamNames, paramNames))
					}
				}
			} else {
				// No params yet, so just add the existing set.
				n.leafParamNames = paramNames
			}
		}

		return n
	}

	c := path[0]
	nextSlash := strings.Index(path, "/")

	var thisToken string
	var tokenEnd int

	if c == '/' {
		// Done processing the previous token, so reset inStaticToken to false.
		thisToken = "/"
		tokenEnd = 1
	} else if nextSlash == -1 {
		thisToken = path
		tokenEnd = len(path)
	} else {
		thisToken = path[0:nextSlash]
		tokenEnd = nextSlash
	}
	remainingPath := path[tokenEnd:]

	if c == '*' && !inStaticToken {
		// Token starts with a *, so it's a catch-all
		thisToken = thisToken[1:]
		if n.catchAllChild == nil {
			n.catchAllChild = &node[T]{path: thisToken, isCatchAll: true}
		}

		if path[1:] != n.catchAllChild.path {
			panic(fmt.Sprintf("treemux: Catch-all name in %s doesn't match %s, You probably tried to define overlapping catchalls.",
				path, n.catchAllChild.path))
		}

		if nextSlash != -1 {
			panic("treemux: / after catch-all found in " + path)
		}

		if paramNames == nil {
			paramNames = []string{thisToken}
		} else {
			paramNames = append(paramNames, thisToken)
		}
		n.catchAllChild.leafParamNames = paramNames

		return n.catchAllChild

	} else if c == '~' && !inStaticToken {
		thisToken = thisToken[1:]
		for _, child := range n.regexChild {
			if thisToken == child.path {
				return child
			}
		}
		re, err := regexp.Compile(thisToken)
		if err != nil {
			panic(fmt.Sprintf("treemux: regular expression %q is invalid: %v", thisToken, err))
		}
		child := &node[T]{path: thisToken, isRegex: true, regExpr: re}
		n.regexChild = append(n.regexChild, child)
		if paramNames == nil {
			paramNames = getRegexParamsNames(re)
		} else {
			paramNames = append(paramNames, getRegexParamsNames(re)...)
		}
		child.leafParamNames = paramNames
		return child

	} else if c == ':' && !inStaticToken {
		// Token starts with a :
		thisToken = thisToken[1:]

		if paramNames == nil {
			paramNames = []string{thisToken}
		} else {
			paramNames = append(paramNames, thisToken)
		}

		if n.wildcardChild == nil {
			n.wildcardChild = &node[T]{path: "wildcard"}
		}

		return n.wildcardChild.addPath(remainingPath, paramNames, false)

	} else {
		// if strings.ContainsAny(thisToken, ":*") {
		// 	panic("* or : in middle of path component " + path)
		// }

		unescaped := false
		if len(thisToken) >= 2 && !inStaticToken {
			if thisToken[0] == '\\' && (thisToken[1] == '*' || thisToken[1] == ':' || thisToken[1] == '~' || thisToken[1] == '\\') {
				// The token starts with a character escaped by a backslash. Drop the backslash.
				c = thisToken[1]
				thisToken = thisToken[1:]
				unescaped = true
			}
		}

		// Set inStaticToken to ensure that the rest of this token is not mistaken
		// for a wildcard if a prefix split occurs at a '*' or ':'.
		inStaticToken = (c != '/')

		// Do we have an existing node that starts with the same letter?
		for i, index := range n.staticIndices {
			if c == index {
				// Yes. Split it based on the common prefix of the existing
				// node and the new one.
				child, prefixSplit := n.splitCommonPrefix(i, thisToken)

				child.priority++
				n.sortStaticChild(i)
				if unescaped {
					// Account for the removed backslash.
					prefixSplit++
				}
				return child.addPath(path[prefixSplit:], paramNames, inStaticToken)
			}
		}

		// No existing node starting with this letter, so create it.
		child := &node[T]{path: thisToken}

		if n.staticIndices == nil {
			n.staticIndices = []byte{c}
			n.staticChild = []*node[T]{child}
		} else {
			n.staticIndices = append(n.staticIndices, c)
			n.staticChild = append(n.staticChild, child)
		}
		return child.addPath(remainingPath, paramNames, inStaticToken)
	}
}

func (n *node[T]) splitCommonPrefix(existingNodeIndex int, path string) (*node[T], int) {
	childNode := n.staticChild[existingNodeIndex]

	if strings.HasPrefix(path, childNode.path) {
		// No split needs to be done. Rather, the new path shares the entire
		// prefix with the existing node, so the new node is just a child of
		// the existing one. Or the new path is the same as the existing path,
		// which means that we just move on to the next token. Either way,
		// this return accomplishes that
		return childNode, len(childNode.path)
	}

	var i int
	// Find the length of the common prefix of the child node and the new path.
	for i = range childNode.path {
		if i == len(path) {
			break
		}
		if path[i] != childNode.path[i] {
			break
		}
	}

	commonPrefix := path[0:i]
	childNode.path = childNode.path[i:]

	// Create a new intermediary node in the place of the existing node, with
	// the existing node as a child.
	newNode := &node[T]{
		path:     commonPrefix,
		priority: childNode.priority,
		// Index is the first letter of the non-common part of the path.
		staticIndices: []byte{childNode.path[0]},
		staticChild:   []*node[T]{childNode},
	}
	n.staticChild[existingNodeIndex] = newNode

	return newNode, i
}

func (n *node[T]) search(method, path string) (found *node[T], handler T, params []string) {
	// if test != nil {
	// 	test.Logf("Searching for %s in %s", path, n.dumpTree("", ""))
	// }

	pathLen := len(path)
	if pathLen == 0 {
		if len(n.leafHandlers) == 0 {
			return
		}
		return n, n.leafHandlers[method], nil
	}

	// First see if this matches a static token.
	firstChar := path[0]
	for i, staticIndex := range n.staticIndices {
		if staticIndex == firstChar {
			child := n.staticChild[i]
			childPathLen := len(child.path)
			if pathLen >= childPathLen && child.path == path[:childPathLen] {
				nextPath := path[childPathLen:]
				found, handler, params = child.search(method, nextPath)
			}
			break
		}
	}

	// If we found a node which has a valid handler, then return here.
	// Otherwise, let's remember that we found this one, but look for a better match.
	if handler.IsValid() {
		return
	}

	if n.wildcardChild != nil {
		// Didn't find a static token, so check for a wildcard.
		nextSlash := strings.IndexByte(path, '/')
		if nextSlash < 0 {
			nextSlash = pathLen
		}

		thisToken := path[0:nextSlash]
		nextToken := path[nextSlash:]

		if len(thisToken) > 0 { // Don't match on empty tokens.
			wcNode, wcHandler, wcParams := n.wildcardChild.search(method, nextToken)
			if wcHandler.IsValid() || (found == nil && wcNode != nil) {
				unescaped, err := unescape(thisToken)
				if err != nil {
					unescaped = thisToken
				}

				if wcParams == nil {
					wcParams = []string{unescaped}
				} else {
					wcParams = append(wcParams, unescaped)
				}

				if wcHandler.IsValid() {
					return wcNode, wcHandler, wcParams
				}

				// Didn't actually find a handler here, so remember that we
				// found a node but also see if we can fall through to the
				// catchall.
				found, handler, params = wcNode, wcHandler, wcParams
			}
		}
	}

	var reNode *node[T]
	var reParams []string
	if len(n.regexChild) > 0 {
		// Test regex routes in their registering order.
		reNode, handler, reParams = n.searchRegexChild(method, path)
		if handler.IsValid() {
			return reNode, handler, reParams
		}
	}

	catchAllChild := n.catchAllChild
	if catchAllChild != nil {
		// Hit the catchall, so just assign the whole remaining path if it
		// has a matching handler.
		handler = catchAllChild.leafHandlers[method]
		// Found a handler, or we found a catchall node without a handler.
		// Either way, return it since there's nothing left to check after this.
		if handler.IsValid() || (found == nil && reNode == nil) {
			unescaped, err := unescape(path)
			if err != nil {
				unescaped = path
			}

			return catchAllChild, handler, []string{unescaped}
		}
	}

	// In case we found a child node without corresponding method handler,
	// return the child node, return it.
	if found != nil {
		return found, handler, params
	}
	return reNode, handler, reParams
}

func (n *node[T]) searchRegexChild(method, path string) (found *node[T], handler T, params []string) {
	for _, child := range n.regexChild {
		re := child.regExpr
		match := re.FindStringSubmatch(path)
		if len(match) == 0 {
			continue
		}

		handler = child.leafHandlers[method]
		if handler.IsValid() {
			return child, handler, match
		}

		// No handler is registered for this method, we return the
		// regex node and params. In case no catchall handler matches,
		// report 405 instead of 404.
		found, params = child, match
		break
	}
	return
}

func (n *node[_]) dumpTree(prefix, nodeType string) string {
	methods := getSortedKeys(n.leafHandlers)
	line := fmt.Sprintf("%s %02d %s%s [%d] %v params %v\n",
		prefix, n.priority, nodeType, n.path, len(n.staticChild), methods, n.leafParamNames)
	prefix += "  "
	for _, node := range n.staticChild {
		line += node.dumpTree(prefix, "")
	}
	if n.wildcardChild != nil {
		line += n.wildcardChild.dumpTree(prefix, ":")
	}
	for _, child := range n.regexChild {
		line += child.dumpTree(prefix, "~")
	}
	if n.catchAllChild != nil {
		line += n.catchAllChild.dumpTree(prefix, "*")
	}
	return line
}
