/*
Package hamt implements a functional Hash Array Mapped Trie (HAMT). Functional
is defined as immutable and persistent. FIXME more explanation.

A Hash Array Mapped Trie (HAMT) is a data structure to map a key (a byte slice)
to value (a interface{}; for a generic item). To do this we use a Trie data-
structure; where each node has a table of SIZE sub-nodes; where each sub-node is
a new table or a leaf; leaf's contain the key/value pairs.

We take a byte slice key and hash it into a 64 bit value. We split the hash value
into ten 6-bit values. Each 6-bit values is used as an index into a 64 entry
table for each node in the Trie structure. For each index, if there is no
collision (ie. no previous entry) in that Trie nodes table's position, then we
put a leaf (aka a value entry); if there is a collision, then we put a new node
table of the Trie structure and use the next 6-bits of the hash value to
calculate the index into that table. This means that the Trie is at most ten
levels deap, AND only as deep as is needed; for savings in memory and access
time. Algorithmically, this allows for a O(1) hash table.

We use go's "hash/fnv" FNV1 implementation for the hash.

Typically HAMT's can be implemented in 64/6 bit and 32/5 bit versions. I've
implemented this as a 64/6 bit version.
*/
package hamt

import (
	"fmt"
	"hash/fnv"
	"log"
	"os"
	"strings"
)

var lgr = log.New(os.Stderr, "[hamt] ", log.Lshortfile)

// The number of bits to partition the hashcode and to index each table. By
// logical necessity this MUST be 6 bits because 2^6 == 64; the number of
// entries in a table.
const NBITS uint = 6

// The Capacity of a table; 2^6 == 64;
const TABLE uint = 1 << NBITS

const sixtyBitMask = 1<<60 - 1

// The maximum depthof a HAMT ranges between 0 and 9, for 10 levels
// total and cunsumes 60 of the 64 bit hashcode.
const MAXDEPTH uint = 9

const ASSERT_CONST bool = true

func ASSERT(test bool, msg string) {
	if ASSERT_CONST {
		if !test {
			panic(msg)
		}
	}
}

// This function calcuates a 64-bit unsigned integer hashcode from an
// arbitrary list of bytes. It uses the FNV1 hashing algorithm:
//   https://golang.org/pkg/hash/fnv/
//   https://en.wikipedia.org/wiki/Fowler%E2%80%93Noll%E2%80%93Vo_hash_function
//
func hash64(bs []byte) uint64 {
	var h = fnv.New64()
	h.Write(bs)
	return h.Sum64()
}

func hashPathEqual(depth uint, a, b uint64) bool {
	pathMask := uint64(1<<(depth*NBITS)) - 1

	return (a & pathMask) == (b & pathMask)
}

func makeHashPath(depth uint, hash uint64) uint64 {
	return hash & hashPathMask(depth)
}

func hashPathMask(depth uint) uint64 {
	return uint64(1<<(depth*NBITS)) - 1
}

func hashPathString(hashPath uint64, depth uint) string {
	var strs = make([]string, depth+1)
	//lgr.Printf("hashPathString: len(strs)=%d; cap(strs)=%d;", len(strs), cap(strs))

	for i := int(depth); i >= 0; i-- {
		var ui = uint(i)
		var idx = index(hashPath, ui)
		//lgr.Printf("strs[%d] = fmt.Sprintf(\"%%06b\", %d)", ui, idx)
		strs[i] = fmt.Sprintf("%06b", idx)
	}
	return strings.Join(strs, " ")
}

func nodeMapString(nodeMap uint64) string {
	var strs = make([]string, 7)

	//lgr.Printf("nodeMapString: BitCount64(nodeMap) = %d", BitCount64(nodeMap))

	var top4 = nodeMap >> 60
	strs[0] = fmt.Sprintf("%04b", top4)

	const tenBitMask uint64 = 1<<10 - 1
	for i := int(0); i < 6; i++ {
		var ui = uint(i)
		//lgr.Printf("i=%d; tenBitMask : %064b", ui, tenBitMask<<(ui*10))
		tenBitVal := (nodeMap & (tenBitMask << (ui * 10))) >> (ui * 10)
		strs[6-ui] = fmt.Sprintf("%010b", tenBitVal)
	}

	return strings.Join(strs, " ")
}

// The hash60Equal function compares the significant 60 bits of hash that
// HAMT uses for navagating the Trie.
//
func hash60Equal(a, b uint64) bool {
	return (a & sixtyBitMask) == (b & sixtyBitMask)
}

//idxMask() generates a NBITS(6-bit) mask for a given depth
func idxMask(depth uint) uint64 {
	var m = uint64(uint8(1<<NBITS) - 1)
	return m << (depth * NBITS)
}

//index() calculates a NBITS(6-bit) integer based on the hash and depth
func index(hash uint64, depth uint) uint {
	var m = idxMask(depth)
	var idx = uint((hash & m) >> (depth * NBITS))
	return idx
}

//buildHashPath()
func buildHashPath(hashPath uint64, depth, idx uint) uint64 {
	if (hashPath&(1<<(depth*NBITS)) - 1) > 0 {
		panic("hashPath > depth")
	}

	return hashPath & uint64(idx<<(depth*NBITS))
}

type Hamt struct {
	root     tableI
	nentries uint
}

var EMPTY = Hamt{nil, 0}

func (h Hamt) String() string {
	return h.root.String()
}

func (h Hamt) IsEmpty() bool {
	return h.root == nil
}

func (h Hamt) copyUp(oldTable, newTable tableI, path pathT) Hamt {
	if path.isEmpty() {
		if h.root != oldTable {
			panic("WTF! path.isEmpty() and h.root != oldTable")
		}
		return Hamt{newTable, 0}
	}

	oldParent := path.pop()

	//newParent := oldParent.replace(oldNode, newNode)
	newParent := oldParent.set(oldTable.hashcode(), newTable)

	return h.copyUp(oldParent, newParent, path)
}

func (h Hamt) copyUpLeaf(oldLeaf, newLeaf leafI, path pathT) Hamt {
	if path.isEmpty() {
		panic("copyUpLeaf() call where path.isEmpty()")
	}

	var oldTable = path.pop()
	var newTable = oldTable.set(oldLeaf.hashcode(), newLeaf)

	return h.copyUp(oldTable, newTable, path)
}

// Get(key) retrieves the value for a given key from the Hamt. The bool
// represents whether the key was found.
func (h Hamt) Get(key []byte) (interface{}, bool) {
	if h.IsEmpty() {
		return nil, false
	}

	lgr.Println("h = ", h)

	var hash = hash64(key)
	var hash60 = hash & sixtyBitMask

	lgr.Printf("key    : %s", key)
	lgr.Printf("hash   : 0x%016x", hash)
	lgr.Printf("hash60 : 0x%016x", hash60)

	// We know h.root != nil (above IsEmpty test) and h.root is a tableI
	// intrface compliant struct.
	var curTable = h.root
	var curNode = curTable.get(hash60)

	lgr.Printf("curNode = %v", curNode)

	for depth := uint(0); curNode != nil && depth <= MAXDEPTH; depth++ {
		//if curNode ISA leafI
		if leaf, ok := curNode.(leafI); ok {
			if hashPathEqual(depth, hash60, leaf.hashcode()) {
				return leaf.get(key)
			}
			return nil, false
		}

		//else curNode MUST BE A tableI
		var curTable = curNode.(tableI)

		curNode = curTable.get(hash60)
	}

	// curNode == nil
	return nil, false
}

func (h Hamt) Put(key []byte, val interface{}) (newHamt Hamt, newVal bool) {
	newHamt = Hamt{nil, 0} //always creating a new Hamt
	newVal = true          //true == inserted key/val pair; false == replaced val

	//var depth uint = 0
	var hash60 = hash64(key) & sixtyBitMask

	/**********************************************************
	 * If the Trie root is empty, insert a table with a leaf. *
	 **********************************************************/
	if h.IsEmpty() {
		leaf := NewFlatLeaf(hash60, key, val)
		newHamt.root = NewCompressedTable(0, hash60, leaf)
		return
	}

	var path = newPathT()
	var curTable = h.root
	var depth uint
	var newLeaf = NewFlatLeaf(hash60, key, val)

	for depth = 0; depth < MAXDEPTH; depth++ {

		var curNode = curTable.get(hash60)

		/***********************************************
		 * If the table entry is empty, insert a leaf. *
		 ***********************************************/
		if curNode == nil {
			var newTable = curTable.set(hash60, newLeaf)
			h.copyUp(curTable, newTable, path)
			return
		}

		/**********************************
		 * If the table entry ISA leaf... *
		 **********************************/
		if oldLeaf, ok := curNode.(leafI); ok {

			/**************************************************************
			 * ...AND hashcode COLLISION. Either we are part of
			 * the way down to MAXDEPTH and part of the hashcode matches,
			 * or we are at MAXDEPTH and 60 of 64 bits match.
			 **************************************************************/
			if hash60Equal(oldLeaf.hashcode(), hash60) {
				//if oldLeaf is a flatLeaf this will promote it to a
				// collisionLeaf, else it already is a collisionLeaf.
				path.push(curTable)
				var newLeaf leafI
				newLeaf, newVal = oldLeaf.put(key, val)
				newHamt.copyUpLeaf(oldLeaf, newLeaf, path)
				return
			}

			var newLeaf = NewFlatLeaf(hash60, key, val)

			// Two leafs into one slot requires a new Table with those two leafs
			// to be put into that slot.
			var hashPath = hash60 & hashPathMask(depth)

			// Table from the collision of two leaves
			colTable := NewCompressedTable2(depth, hashPath, oldLeaf, newLeaf)

			newTable := curTable.set(hash60, colTable)

			h.copyUp(curTable, newTable, path)
			return
		}

		path.push(curTable)

		/***********************************
		 * The tale entry MUST BE A tableI *
		 ***********************************/
		curTable = curNode.(tableI)

	} //end: for

	return newHamt, false
}

func (h Hamt) Del(key []byte) (Hamt, interface{}, bool) {
	return Hamt{nil, 0}, nil, false
}

// The nodeI interface can be compressedTable, fullTable, flatLeaf, or
// collisionLeaf
//
// For table structs hashcode() is the depth*NBITS of the hash path that leads
// to thistable's position in the Trie.
//
// For leafs hashcode() is the top 60 bits out of the hash64(key).
// For collisionLeafs this is the definition of what a collision is.
//
type nodeI interface {
	hashcode() uint64
	String() string
}

type tableI interface {
	nodeI
	copy() tableI
	entries() []nodeI
	nentries() uint
	get(hash uint64) nodeI
	set(hash uint64, entry nodeI) tableI
	del(hash uint64) (tableI, nodeI)
}

type compressedTable struct {
	hashPath uint64 // depth*NBITS of hash to get to this location in the Trie
	depth    uint
	nodeMap  uint64
	nodes    []nodeI
}

// Utility structure only used for NewCompressedTable() constructor.
//
type tableEntry struct {
	idx  uint
	hash uint64
	key  []byte
	val  interface{}
}

type tableS []tableEntry

func NewCompressedTable(depth uint, hashPath uint64, lf leafI) tableI {
	ASSERT(depth < MAXDEPTH+1, "uint parameter 0 >= depth >= 9")

	var idx = index(lf.hashcode(), depth)

	var ct = new(compressedTable)
	ct.hashPath = hashPath
	ct.depth = depth
	ct.nodeMap = 1 << idx
	ct.nodes = make([]nodeI, 1)
	ct.nodes[0] = lf

	return ct
}

func NewCompressedTable2(depth uint, hashPath uint64, leaf1 leafI, leaf2 flatLeaf) tableI {
	ASSERT(depth < MAXDEPTH+1, "uint parameter 0 >= depth >= 9")

	var retTable = new(compressedTable) // return compressedTable
	retTable.hashPath = hashPath
	retTable.depth = depth

	var curTable = retTable //*compressedTable
	var d uint
	for d = depth; d < MAXDEPTH; d++ {
		var idx1 = index(leaf1.hashcode(), d)
		var idx2 = index(leaf2.hashcode(), d)
		curTable.nodes = make([]nodeI, 2)

		if idx1 != idx2 {
			curTable.nodeMap &= 1 << idx1
			curTable.nodeMap &= 1 << idx2
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
		var newTable = new(compressedTable)
		//newTable.hashPath = buildHashPath(hashPath, d+1, idx1)
		newTable.hashPath = hashPath & uint64(idx1<<((d+1)*NBITS))
		newTable.depth = d + 1

		curTable.nodeMap = 1 << idx1
		curTable.nodes = append(curTable.nodes, newTable)

		curTable = newTable
	}
	if d == MAXDEPTH {
		// leaf1.hashcode() == leaf2.hashcode()
		leaf, _ := leaf1.put(leaf2.key, leaf2.val)
		curTable.set(leaf.hashcode(), leaf)
	}

	return retTable
}

func (t compressedTable) hashcode() uint64 {
	return t.hashPath
}

func (t compressedTable) copy() tableI {
	var nt = new(compressedTable)
	nt.hashPath = t.hashPath
	nt.depth = t.depth
	nt.nodeMap = t.nodeMap
	nt.nodes = append(nt.nodes, t.nodes...)
	return nt
}

func (t compressedTable) String() string {
	var strs = make([]string, 2+len(t.nodes))
	// Sprintf depth hashPath
	strs[0] = fmt.Sprintf("%d %s", t.depth, hashPathString(t.hashPath, t.depth))

	// Sprintf 0b64 of nodeMap
	strs[1] = nodeMapString(t.nodeMap)

	//strs[2] = fmt.Sprintf("len(t.nodes) = %d", len(t.nodes))

	// for each node in nodes
	//     String node
	for i, n := range t.nodes {
		strs[2+i] = n.String()
	}

	return strings.Join(strs, "\n")
}

func (t compressedTable) entries() []nodeI {
	return nil
}

func (t compressedTable) nentries() uint {
	return BitCount64(t.nodeMap)
}

func (t compressedTable) get(hash uint64) nodeI {
	// Get the regular index of the node we want to access
	var idx = index(hash, t.depth) // 0..63
	var nodeBit = uint64(1 << idx)

	if (t.nodeMap & nodeBit) == 0 {
		return nil
	}

	// Create a mask to mask off all bits below idx'th bit
	var m = uint64(1<<idx) - 1

	// Count the number of bits in the nodeMap below the idx'th bit
	var i = BitCount64(t.nodeMap & m)

	var node = t.nodes[i]

	return node
}

func (t compressedTable) set(hash uint64, newNode nodeI) tableI {
	var nt = t.copy().(*compressedTable)

	// Get the regular index of the node we want to access
	var idx = index(hash, t.depth) // 0..63
	var bit = uint64(1 << idx)     //is the slot

	var bitMask = uint64(1<<idx) - 1

	var i = BitCount64(t.nodeMap & bitMask)

	if (t.nodeMap & bit) == 0 {
		// slot is empty, so put nodeI in slot
		nt.nodeMap &= bit

		//insert newnode into the i'th spot of t.nodes
		nt.nodes = append(nt.nodes[:i], append([]nodeI{newNode}, nt.nodes[i:]...)...)

		return nt
	}

	// slot is used
	//
	//var oldNode = t.nodes[i]

	nt.nodes[i] = newNode

	return nt
}

func (t compressedTable) del(hash uint64) (tableI, nodeI) {
	return nil, nil
}

type fullTable struct {
	hashPath uint64 // depth*NBITS of hash to get to this location in the Trie
	depth    uint
	nodeMap  uint64
	nodes    [TABLE]nodeI
}

func NewFullTable(depth uint, hash uint64, nt tableS) tableI {
	return nil
}

func (t fullTable) hashcode() uint64 {
	return t.hashPath
}

func (t fullTable) copy() tableI {
	return nil
}

func (t fullTable) entries() []nodeI {
	return nil
}

func (t fullTable) get(hash uint64) nodeI {
	return nil
}

func (t fullTable) set(hash uint64, entry nodeI) tableI {
	return nil
}

func (t fullTable) del(hash uint64) (tableI, nodeI) {
	return nil, nil
}

func (t fullTable) nentries() uint {
	return BitCount64(t.nodeMap)
}

type leafI interface {
	nodeI
	get(key []byte) (interface{}, bool)
	put(key []byte, val interface{}) (leafI, bool) //bool == replace? val
	del(key []byte) (leafI, interface{}, bool)     //bool == deleted? key
}

type flatLeaf struct {
	hash60 uint64 //hash64(key) & sixtyBitMask
	key    []byte
	val    interface{}
}

func NewFlatLeaf(hash uint64, key []byte, val interface{}) flatLeaf {
	var hash60 uint64 = hash & sixtyBitMask
	var leaf = flatLeaf{hash60, key, val}
	return leaf
}

func (l flatLeaf) hashcode() uint64 {
	return l.hash60
}

func (l flatLeaf) String() string {
	// flatLeaf{hash60:0x0123456789abcde01, key:"foo", val:1}
	return fmt.Sprintf("flatLeaf{hash60:0x%016x, key:[]byte(\"%s\"), val:%v}", l.hash60, l.key, l.val)
}

func (l flatLeaf) get(key []byte) (interface{}, bool) {
	if byteSlicesEqual(l.key, key) {
		return l.val, true
	}
	return nil, false
}

//
func (l flatLeaf) put(key []byte, val interface{}) (leafI, bool) {
	if byteSlicesEqual(l.key, key) {
		hash60 := hash64(key) & sixtyBitMask
		nl := NewFlatLeaf(hash60, key, val)
		return nl, true
	}

	var nl = new(collisionLeaf)
	nl.hash60 = l.hashcode()
	nl.kvs = append(nl.kvs, keyVal{l.key, l.val})
	nl.kvs = append(nl.kvs, keyVal{key, val})
	return nl, false //didn't replace
}

func (l flatLeaf) del(key []byte) (leafI, interface{}, bool) {
	if byteSlicesEqual(l.key, key) {
		return nil, l.val, true //deleted entry
	}
	return nil, nil, false //didn't delete
}

type keyVal struct {
	key []byte
	val interface{}
}

func (kv keyVal) String() string {
	return fmt.Sprintf("{[]byte(\"%s\"), val:%v}", kv.key, kv.val)
}

type collisionLeaf struct {
	hash60 uint64 //hash64(key) & sixtyBitMask
	kvs    []keyVal
}

func NewCollisionLeaf(hash uint64, kvs []keyVal) *collisionLeaf {
	leaf := new(collisionLeaf)
	leaf.hash60 = hash & sixtyBitMask
	leaf.kvs = append(leaf.kvs, kvs...)

	return leaf
}

func (l collisionLeaf) hashcode() uint64 {
	return l.hash60
}

func (l collisionLeaf) String() string {
	var kvstrs = make([]string, len(l.kvs))
	for i := 0; i < len(l.kvs); i++ {
		kvstrs[i] = l.kvs[i].String()
	}
	var kvstr = strings.Join(kvstrs, ",")
	return fmt.Sprintf("{hash60:%016x, kvs:[]kv{%s}}", l.hash60, kvstr)
}

func (l collisionLeaf) get(key []byte) (interface{}, bool) {
	for i := 0; i < len(l.kvs); i++ {
		if byteSlicesEqual(l.kvs[i].key, key) {
			return l.kvs[i].val, true
		}
	}
	return nil, false
}

func (l collisionLeaf) put(key []byte, val interface{}) (leafI, bool) {
	nl := new(collisionLeaf)
	nl.hash60 = l.hash60
	nl.kvs = append(nl.kvs, l.kvs...)

	for i, kv := range l.kvs {
		if byteSlicesEqual(kv.key, key) {
			nl.kvs[i].val = val
			return nl, true // val was replaced
		}
	}

	nl.kvs = append(nl.kvs, keyVal{key, val})
	return nl, false
}

func (l collisionLeaf) del(key []byte) (leafI, interface{}, bool) {
	if len(l.kvs) == 2 {
		if byteSlicesEqual(key, l.kvs[0].key) {
			return NewFlatLeaf(l.hash60, l.kvs[1].key, l.kvs[1].val), l.kvs[0].val, true
		}
		if byteSlicesEqual(key, l.kvs[1].key) {
			return NewFlatLeaf(l.hash60, l.kvs[0].key, l.kvs[0].val), l.kvs[1].val, true
		}
		return nil, nil, false
	}

	var retVal interface{} = nil
	var removed bool = false
	var nl = new(collisionLeaf)

	nl.hash60 = l.hash60

	for i := 0; i < len(l.kvs); i++ {
		if !byteSlicesEqual(key, l.kvs[i].key) {
			nl.kvs = append(nl.kvs, l.kvs[i])
		} else {
			retVal = l.kvs[i].val
			removed = true
		}
	}

	return nl, retVal, removed
}

//byteSlicesEqual function
//
func byteSlicesEqual(k1 []byte, k2 []byte) bool {
	if len(k1) != len(k2) {
		return false
	}
	if len(k1) == 0 {
		return true
	}
	for i := 0; i < len(k1); i++ {
		if k1[i] != k2[i] {
			return false
		}
	}
	return true
}

//POPCNT Implementation
//

const (
	HEXI_FIVES  = uint64(0x5555555555555555)
	HEXI_THREES = uint64(0x3333333333333333)
	HEXI_ONES   = uint64(0x0101010101010101)
	HEXI_FS     = uint64(0x0f0f0f0f0f0f0f0f)
)

func BitCount64(n uint64) uint {
	n = n - ((n >> 1) & HEXI_FIVES)
	n = (n & HEXI_THREES) + ((n >> 2) & HEXI_THREES)
	return uint((((n + (n >> 4)) & HEXI_FS) * HEXI_ONES) >> 56)
}
