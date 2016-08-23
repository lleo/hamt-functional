
SYNOPSIS
========

This library is a Go language implementation of a functional HAMT, Hash Array
Mapped Trie.

There is a lot of Jargon in there to be unpacked. This library can be imported
into a Go language program with the following golang import statements:

	import hamt "github.com/lleo/go-hamt-functional"

The term, functional means "immutable" and "persistent". The term "immutable"
means that the datastructure is constant, ie it is never modified. The term
"persistent" means that any only the changes to the parent datastructure are
added and all unchanged parts, of the parent datastructure, are shared with
the child. In other words, a new top level data structure is created which
only contains the differences from the original, yet shares all the unmodified
parts of the original datastructure. Say you have a Tree where you add a leaf.
The new leaf is added to a modified internal modified tree node. That internal
modified tree node is a copy of the original tree node. And each tree node
parent upto the root tree node is copied and modified. All other nodes in the
datastructure remain "persistent" and unchanged.

Imagine a hypothetical tree structure with four leaves, two interior nodes and
a root node. If you change the fourth leaf node, then a new fourth leaf node
is created, as well as it's parent interior node, and a new root node.

	        root tree node   root tree node'
	            /    \         /   \   	 	
	           /  +-- \----- +      \ 	   	   	
              /  /     \             \
	   tree node 1   tree node 2  tree node 2'
	  	  /  \          /  \        /   \
	     /    \        / +--\------+     \
        /  	   \   	  /	/  	 \ 	   	   	  \
	Leaf 1	Leaf 2  Leaf 3  Leaf 4     Leaf 4'

Given this approach to changing a tree, a tree with a wide branching factor
would be relatively shallow. So the path from root to leaf would be short and
the amount of shared content would be substantial.

A Hash Array Mapped Trie is Trie where each node is represented by an Array.
The pointer to the next node down in the Trie is selected by and index drived
from Hash of the Key. For a 32 bit hash value we use Arrays 32 entries deep and the
hash value is chopped into 6 groups of 5 bits each. For a 64 bit hash value,
we use Arrays 64 entries deep and the hash value is chopped into 10 groups of
6 bits each. Each bit group is the index into the next table; for 32 bit hash
values 2^5 == 32; for 64 bit hash values 2^6 == 64.


