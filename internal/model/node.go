package model

// Ref points at another book, node or block. A link entry carries one.
type Ref struct {
	Book  BookID `json:"book,omitempty"`
	Node  ID     `json:"node,omitempty"`
	Block ID     `json:"block,omitempty"`
}

// Valid reports whether the reference points at something.
func (ref *Ref) Valid() bool {
	if ref == nil {
		return false
	}
	return ref.Book != "" || !ref.Node.IsZero() || !ref.Block.IsZero()
}

// Node is one structural level or one link entry. It carries no body: blocks are the only bodies.
type Node struct {
	ID       ID      `json:"id"`
	Kind     Kind    `json:"kind"`
	Title    string  `json:"title,omitempty"`
	Part     string  `json:"part,omitempty"`
	Blocks   []ID    `json:"blocks,omitempty"`
	Children []*Node `json:"children,omitempty"`
	Ref      *Ref    `json:"ref,omitempty"`
}

// Clone returns a deep copy.
func (node *Node) Clone() *Node {
	if node == nil {
		return nil
	}
	clone := &Node{
		ID:    node.ID,
		Kind:  node.Kind,
		Title: node.Title,
		Part:  node.Part,
	}
	if node.Blocks != nil {
		clone.Blocks = append([]ID(nil), node.Blocks...)
	}
	if node.Ref != nil {
		ref := *node.Ref
		clone.Ref = &ref
	}
	if node.Children != nil {
		clone.Children = make([]*Node, len(node.Children))
		for index, child := range node.Children {
			clone.Children[index] = child.Clone()
		}
	}
	return clone
}

// Walk visits this node and its descendants depth-first. The callback receives the depth starting
// at one and the parent node. Returning false skips the node's children.
//
// The explicit depth parameter exists because a (node, parent) pair being mistaken for a depth is
// a defect class the reference implementation hit.
func (node *Node) Walk(visit func(current *Node, depth int, parent *Node) bool) {
	if node == nil {
		return
	}
	walk(node, 1, nil, visit)
}

func walk(node *Node, depth int, parent *Node, visit func(*Node, int, *Node) bool) {
	if node == nil {
		return
	}
	if !visit(node, depth, parent) {
		return
	}
	for _, child := range node.Children {
		walk(child, depth+1, node, visit)
	}
}
