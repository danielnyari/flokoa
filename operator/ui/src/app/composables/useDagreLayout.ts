import dagre from 'dagre'
import type { Node, Edge } from '@vue-flow/core'

export function useDagreLayout() {
  function layout(nodes: Node[], edges: Edge[], direction: 'TB' | 'LR' = 'TB'): Node[] {
    const g = new dagre.graphlib.Graph()
    g.setDefaultEdgeLabel(() => ({}))
    g.setGraph({
      rankdir: direction,
      nodesep: 60,
      ranksep: 80,
      marginx: 20,
      marginy: 20
    })

    for (const node of nodes) {
      g.setNode(node.id, {
        width: node.type === 'root' ? 60 : node.type === 'switch' ? 80 : 200,
        height: node.type === 'root' ? 60 : node.type === 'switch' ? 80 : 80
      })
    }

    for (const edge of edges) {
      g.setEdge(edge.source, edge.target)
    }

    dagre.layout(g)

    return nodes.map((node) => {
      const pos = g.node(node.id)
      const width = node.type === 'root' ? 60 : node.type === 'switch' ? 80 : 200
      const height = node.type === 'root' ? 60 : node.type === 'switch' ? 80 : 80
      return {
        ...node,
        position: {
          x: pos.x - width / 2,
          y: pos.y - height / 2
        }
      }
    })
  }

  return { layout }
}
