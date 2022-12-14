package treemux

// Params contains the parameters matched from request path, as returned by the router.
// The slice is ordered, the first URL parameter is also the first slice value.
// It is therefore safe to read values by the index.
type Params struct {
	Keys   []string
	Values []string
}

// Get returns the value of the param which matches name.
// If no matching pram is found, an empty string is returned.
func (ps *Params) Get(name string) string {
	for i, key := range ps.Keys {
		if key == name {
			return ps.Values[i]
		}
	}
	return ""
}

// Append appends a new key value pair to Params.
func (ps *Params) Append(key, value string) {
	ps.Keys = append(ps.Keys, key)
	ps.Values = append(ps.Values, value)
}
