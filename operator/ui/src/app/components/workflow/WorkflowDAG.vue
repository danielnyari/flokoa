<script setup lang="ts">
import { nextTick, computed } from 'vue'
import { VueFlow, useVueFlow } from '@vue-flow/core'
import type { Node, Edge } from '@vue-flow/core'
import type { WorkflowRun, WorkflowRunNode } from '~/types'
import { isRunPhase } from '~/utils/enums'
import AgentNode from './nodes/AgentNode.vue'
import RootNode from './nodes/RootNode.vue'
import SwitchNode from './nodes/SwitchNode.vue'

const props = defineProps<{
  run: WorkflowRun
  tasks?: { name: string, type: string }[]
}>()

const selectedNode = defineModel<string | null>('selectedNode')

const { fitView } = useVueFlow()
const { layout } = useDagreLayout()

function nodeDuration(node: WorkflowRunNode): string | undefined {
  if (!node.startedAt) return undefined
  const start = new Date(node.startedAt)
  const end = node.finishedAt ? new Date(node.finishedAt) : new Date()
  const seconds = Math.floor((end.getTime() - start.getTime()) / 1000)
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const remaining = seconds % 60
  return `${minutes}m ${remaining}s`
}

function getNodeType(node: WorkflowRunNode): string {
  // Check the task type from workflow spec
  const task = props.tasks?.find(t => t.name === node.templateName)
  if (task?.type === 'switch') return 'switch'

  // DAG root nodes
  if (node.type === 'NODE_TYPE_DAG') return 'root'

  // Skip retry/task-group/steps (structural nodes)
  // Plugin nodes are agent tasks
  return 'agent'
}

function shouldShowNode(node: WorkflowRunNode): boolean {
  // Hide retry wrapper nodes and step group nodes
  if (node.type === 'NODE_TYPE_RETRY') return false
  if (node.type === 'NODE_TYPE_TASK_GROUP') return false
  if (node.type === 'NODE_TYPE_STEPS') return false
  return true
}

const graphData = computed(() => {
  const runNodes = props.run.nodes ?? []
  const visibleNodes = runNodes.filter(shouldShowNode)

  // Build node id map for resolving children through hidden nodes
  const nodeMap = new Map(runNodes.map(n => [n.id, n]))

  // Resolve children, skipping over hidden intermediate nodes
  function resolveChildren(nodeId: string): string[] {
    const node = nodeMap.get(nodeId)
    if (!node?.children) return []
    const resolved: string[] = []
    for (const childId of node.children) {
      const child = nodeMap.get(childId)
      if (child && shouldShowNode(child)) {
        resolved.push(childId)
      } else if (child) {
        // recurse through hidden node
        resolved.push(...resolveChildren(childId))
      }
    }
    return resolved
  }

  const nodes: Node[] = visibleNodes.map((node) => {
    const nodeType = getNodeType(node)
    return {
      id: node.id,
      type: nodeType,
      position: { x: 0, y: 0 },
      data: {
        label: node.displayName || node.name,
        phase: node.phase,
        duration: nodeDuration(node),
        selected: selectedNode.value === node.id
      }
    }
  })

  const edges: Edge[] = []
  for (const node of visibleNodes) {
    const children = resolveChildren(node.id)
    for (const childId of children) {
      const childNode = nodeMap.get(childId)
      const isRunning = isRunPhase(childNode?.phase, 'Running')
      edges.push({
        id: `${node.id}->${childId}`,
        source: node.id,
        target: childId,
        animated: isRunning
      })
    }
  }

  // Apply dagre layout
  const positioned = layout(nodes, edges, 'TB')
  return { nodes: positioned, edges }
})

function onNodesInitialized() {
  nextTick(() => {
    fitView()
  })
}

function onNodeClick(event: { node: Node }) {
  selectedNode.value = selectedNode.value === event.node.id ? null : event.node.id
}
</script>

<template>
  <div class="h-full w-full">
    <VueFlow
      :nodes="graphData.nodes"
      :edges="graphData.edges"
      :nodes-connectable="false"
      :nodes-draggable="true"
      fit-view-on-init
      class="h-full"
      @nodes-initialized="onNodesInitialized"
      @node-click="onNodeClick"
    >
      <template #node-agent="agentProps">
        <AgentNode v-bind="agentProps" />
      </template>
      <template #node-root="rootProps">
        <RootNode v-bind="rootProps" />
      </template>
      <template #node-switch="switchProps">
        <SwitchNode v-bind="switchProps" />
      </template>
    </VueFlow>
  </div>
</template>

<style>
@import '@vue-flow/core/dist/style.css';
@import '@vue-flow/core/dist/theme-default.css';
</style>
