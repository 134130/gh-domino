package gitobj

type Commit struct {
	Oid     string `json:"oid"`
	Message string `json:"message"`
}
