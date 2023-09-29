package trie

import (
	"fmt"
	"../modules/logger"
	"github.com/sirupsen/logrus"
)

/*
This is a trie which maps topic subscriptions with subscribed users.
*/

type node[T any] struct {
	UserIDs  map[string]*T
	Letter   rune
	Previous *node[T]
	Children map[rune]*node[T]
}

type nodeChild[T any] struct {
	n    *node[T]
	next *nodeChild[T]
}

type Trie[T any] struct {
	root *node[T]
}

func New[T any]() *Trie[T] {
	return &Trie[T]{
		root: &node[T]{
			UserIDs:  make(map[string]*T),
			Children: make(map[rune]*node[T]),
		},
	}
}

func (t *Trie[T]) Remove(name, userID string) *T {
	if name == "" {
		return nil
	}

	var current = t.getTopicNode(name)

	if current == nil {
		return nil
	}

	subscription, _ := current.UserIDs[userID]
	delete(current.UserIDs, userID)
	t.cleanTopicPath(current)

	return subscription
}

func (t *Trie[T]) Add(name, userID string, data *T) {
	if name == "" {
		return
	}

	var current = t.root
	var letter rune

	for _, letter = range name {
		var newLetterNode = t.createLetterNode(letter, current)
		newLetterNode.Previous = current
		current = t.addLetterToNodeChildren(newLetterNode, current)
	}

	current.UserIDs[userID] = data
}

func (t *Trie[T]) Get(name string) map[string]*T {
	var topicNode = t.getTopicNode(name)

	if topicNode == nil {
		return nil
	}

	return topicNode.UserIDs
}

func (t *Trie[T]) cleanTopicPath(n *node[T]) {
	if t.nodeStillUsed(n) {
		return
	}

	var current = n.Previous

	if current == nil {
		n = nil
		return
	}

	if _, ok := current.Children[n.Letter]; ok {
		delete(current.Children, n.Letter)

		if len(current.Children) == 0 {
			current.Children = nil
		}
	}

	t.cleanTopicPath(current)
}

func (t *Trie[T]) nodeStillUsed(n *node[T]) bool {
	if n == nil {
		return false
	}

	if len(n.Children) != 0 || len(n.UserIDs) != 0 {
		return true
	}

	return false
}

func (t *Trie[T]) getTopicNode(name string) *node[T] {
	var current = t.root
	var letter rune

	for _, letter = range name {
		if current = t.findLetterInNode(letter, current.Children); current == nil {
			return nil
		}
	}

	return current
}

func (t *Trie[T]) createLetterNode(letter rune, n *node[T]) *node[T] {
	if foundNode, ok := n.Children[letter]; ok {
		return foundNode
	}

	return &node[T]{
		Letter:  letter,
		UserIDs: make(map[string]*T),
	}
}

func (t *Trie[T]) findLetterInNode(letter rune, children map[rune]*node[T]) *node[T] {
	if foundNode, ok := children[letter]; ok {
		return foundNode
	}

	return nil
}

func (t *Trie[T]) addLetterToNodeChildren(newLetterNode *node[T], n *node[T]) *node[T] {
	if n == nil || newLetterNode == nil {
		return nil
	}

	if n.Children == nil {
		n.Children = make(map[rune]*node[T])
		n.Children[newLetterNode.Letter] = newLetterNode
		return newLetterNode
	}

	if letterNode := t.findLetterInNode(newLetterNode.Letter, n.Children); letterNode != nil {
		return letterNode
	}

	n.Children[newLetterNode.Letter] = newLetterNode

	return newLetterNode
}
