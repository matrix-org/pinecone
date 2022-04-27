// copyright 2022 the matrix.org foundation c.i.c.
//
// licensed under the apache license, version 2.0 (the "license");
// you may not use this file except in compliance with the license.
// you may obtain a copy of the license at
//
//     http://www.apache.org/licenses/license-2.0
//
// unless required by applicable law or agreed to in writing, software
// distributed under the license is distributed on an "as is" basis,
// without warranties or conditions of any kind, either express or implied.
// see the license for the specific language governing permissions and
// limitations under the license.

package integration

type Node struct {
	name string
	key  string
}

type byKey []Node

func (l byKey) Len() int {
	return len(l)
}

func (l byKey) Less(i, j int) bool {
	return l[i].key < l[j].key
}

func (l byKey) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}
