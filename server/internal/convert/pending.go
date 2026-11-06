// SPDX-License-Identifier: Apache-2.0
package convert

func (q *Queue) Pending(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.pending[id]
}
