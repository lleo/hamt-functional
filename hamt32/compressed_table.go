package hamt32

import (
	"fmt"
	"strings"
)

// The compressedTable is a low memory usage version of a fullTable. It applies
// to tables with less than TABLE_CAPACITY/2 number of entries in the table.
//
// It records which table entries are populated using a bit map called nodeMap.
//
// It stores the nodes in a go slice starting with the node corresponding to
// the Least Significant Bit(LSB) is the first node in the slice. While precise
// and accurate this discription does not help boring regular programmers. Most
// bit patterns are drawn from the Most Significant Bits(MSB) to the LSB; in
// orther words for a uint32 from the 31st bit to the 0th bit left to right. So
// for a 8bit number 1 is writtent as 00000001 (where the LSB is 1) and 128 is
// written as 10000000 (where the MSB is 1).
//
// So the number of entries in the node slice is equal to the number of bits set
// in the nodeMap. You can count the number of bits in the nodeMap, a 32bit word,
// by calculating the Hamming Weight (another obscure name; google it). The
// simple most generice way of calculating the Hamming Weight of a 32bit work is
// implemented in the BitCount32(uint32) function defined bellow.
//
// To figure out the index of a node in the nodes slice from the index of the bit
// in the nodeMap we first find out if that bit in the nodeMap is set by
// calculating if "nodeMap & (1<<idx) > 0" is true the idx'th bit is set. Given
// that each 32 entry table is indexed by 5bit section (2^5==32) of the key hash,
// there is a function to calculate the index called index(hash, depth);
//
type compressedTable struct {
	hashPath uint32 // depth*NBITS of hash to get to this location in the Trie
	nodeMap  uint32
	nodes    []nodeI
}

func newCompressedTable(depth uint, hashPath uint32, lf leafI) tableI {
	var idx = index(hashPath, depth)

	var ct = new(compressedTable)
	ct.hashPath = hashPath & hashPathMask(depth)
	ct.nodeMap = 1 << idx
	ct.nodes = make([]nodeI, 1)
	ct.nodes[0] = lf

	return ct
}

func newCompressedTable2(depth uint, hashPath uint32, leaf1 leafI, leaf2 flatLeaf) tableI {
	var retTable = new(compressedTable)
	retTable.hashPath = hashPath & hashPathMask(depth)

	var curTable = retTable
	var d uint
	for d = depth; d <= MAXDEPTH; d++ {
		var idx1 = index(leaf1.hashcode(), d)
		var idx2 = index(leaf2.hashcode(), d)

		if idx1 != idx2 {
			curTable.nodes = make([]nodeI, 2)

			curTable.nodeMap |= 1 << idx1
			curTable.nodeMap |= 1 << idx2
			if idx1 < idx2 {
				curTable.nodes[0] = leaf1
				curTable.nodes[1] = leaf2
			} else {
				curTable.nodes[0] = leaf2
				curTable.nodes[1] = leaf1
			}

			break
		}
		// idx1 == idx2 && continue

		curTable.nodes = make([]nodeI, 1)

		hashPath = buildHashPath(hashPath, idx1, d)

		var newTable = new(compressedTable)
		newTable.hashPath = hashPath

		curTable.nodeMap = 1 << idx1 //Set the idx1'th bit
		curTable.nodes[0] = newTable

		curTable = newTable
	}
	// We either BREAK out of the loop,
	// OR we hit d > MAXDEPTH.
	if d > MAXDEPTH {
		// leaf1.hashcode() == leaf2.hashcode()
		var idx = index(leaf1.hashcode(), MAXDEPTH)
		leaf, _ := leaf1.put(leaf2.key, leaf2.val)
		curTable.set(idx, leaf)
	}

	return retTable
}

// downgradeToCompressedTable() converts fullTable structs that have less than
// TABLE_CAPACITY/2 tableEntry's. One important thing we know is that none of
// the entries will collide with another.
//
// The ents []tableEntry slice is guaranteed to be in order from lowest idx to
// highest. tableI.entries() also adhears to this contract.
func downgradeToCompressedTable(hashPath uint32, ents []tableEntry) *compressedTable {
	var nt = new(compressedTable)
	nt.hashPath = hashPath
	//nt.nodeMap = 0
	nt.nodes = make([]nodeI, len(ents))

	for i := 0; i < len(ents); i++ {
		var ent = ents[i]
		var nodeBit = uint32(1 << ent.idx)
		nt.nodeMap |= nodeBit
		nt.nodes[i] = ent.node
	}

	return nt
}

func (t compressedTable) hashcode() uint32 {
	return t.hashPath
}

func (t compressedTable) copy() *compressedTable {
	var nt = new(compressedTable)
	nt.hashPath = t.hashPath
	nt.nodeMap = t.nodeMap
	nt.nodes = append(nt.nodes, t.nodes...)
	return nt
}

//String() is required for nodeI depth
func (t compressedTable) String() string {
	// compressedTale{hashPath:/%d/%d/%d/%d/%d/%d/%d/%d/%d/%d, nentries:%d,}
	return fmt.Sprintf("compressedTable{hashPath:%s, nentries()=%d}",
		hash30String(t.hashPath), t.nentries())
}

func (t compressedTable) toString(depth uint) string {
	return fmt.Sprintf("compressedTable{hashPath:%s, nentries()=%d}",
		hashPathString(t.hashPath, depth), t.nentries())
}

// LongString() is required for tableI
func (t compressedTable) LongString(indent string, depth uint) string {
	var strs = make([]string, 3+len(t.nodes))

	strs[0] = indent + fmt.Sprintf("compressedTable{hashPath=%s, nentries()=%d", hashPathString(t.hashPath, depth), t.nentries())

	strs[1] = indent + "\tnodeMap=" + nodeMapString(t.nodeMap) + ","

	for i, n := range t.nodes {
		if t, ok := n.(tableI); ok {
			strs[2+i] = indent + fmt.Sprintf("\tt.nodes[%d]:\n%s", i, t.LongString(indent+"\t", depth+1))
		} else {
			strs[2+i] = indent + fmt.Sprintf("\tt.nodes[%d]: %s", i, n.String())
		}
	}

	strs[len(strs)-1] = indent + "}"

	return strings.Join(strs, "\n")
}

func (t compressedTable) nentries() uint {
	return bitCount32(t.nodeMap)
}

// This function MUST return the slice of tableEntry structs from lowest
// tableEntry.idx to highest tableEntry.idx .
func (t compressedTable) entries() []tableEntry {
	var n = t.nentries()
	var ents = make([]tableEntry, n)

	for i, j := uint(0), uint(0); i < TABLE_CAPACITY; i++ {
		var nodeBit = uint32(1 << i)

		if (t.nodeMap & nodeBit) > 0 {
			ents[j] = tableEntry{i, t.nodes[j]}
			j++
		}
	}

	return ents
}

func (t compressedTable) get(idx uint) nodeI {
	var nodeBit = uint32(1 << idx)

	if (t.nodeMap & nodeBit) == 0 {
		return nil
	}

	// Create a mask to mask off all bits below idx'th bit
	var m = uint32(1<<idx) - 1

	// Count the number of bits in the nodeMap below the idx'th bit
	var i = bitCount32(t.nodeMap & m)

	var node = t.nodes[i]

	return node
}

// set(uint32, nodeI) is required for tableI
func (t compressedTable) set(idx uint, nn nodeI) tableI {
	var nt = t.copy()

	var nodeBit = uint32(1 << idx) // idx is the slot
	var bitMask = nodeBit - 1      // mask all bits below the idx'th bit

	// Calculate the index into compressedTable.nodes[] for this entry
	var i = bitCount32(t.nodeMap & bitMask)

	if nn != nil {
		if (t.nodeMap & nodeBit) == 0 {
			nt.nodeMap |= nodeBit

			// insert newnode into the i'th spot of nt.nodes[]
			nt.nodes = append(nt.nodes[:i], append([]nodeI{nn}, nt.nodes[i:]...)...)

			if bitCount32(nt.nodeMap) >= TABLE_CAPACITY/2 {
				// promote compressedTable to fullTable
				return upgradeToFullTable(nt.hashPath, nt.entries())
			}
		} else /* if (t.nodeMap & nodeBit) > 0 */ {
			// don't need to touch nt.nodeMap
			nt.nodes[i] = nn //overwrite i'th slice entry
		}
	} else /* if nn == nil */ {
		if (t.nodeMap & nodeBit) > 0 {

			nt.nodeMap &^= nodeBit //unset nodeBit via bitClear &^ op
			nt.nodes = append(nt.nodes[:i], nt.nodes[i+1:]...)

			if nt.nodeMap == 0 {
				// FIXME: Is len(nt.nodes) == 0 ?
				return nil
			}
		} else if (t.nodeMap & nodeBit) == 0 {
			// do nothing
			return t
		}
	}

	return nt
}