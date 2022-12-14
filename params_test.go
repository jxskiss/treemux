package treemux

func newParams(kv ...string) Params {
	ps := Params{}
	for i := 0; i < len(kv); i += 2 {
		ps.Keys = append(ps.Keys, kv[i])
		ps.Values = append(ps.Values, kv[i+1])
	}
	return ps
}
