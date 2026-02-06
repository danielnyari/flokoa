# Model Resource

The `Model` resource defines a specific LLM model with its parameters, connecting to a provider through a ModelProvider reference.

## Overview

A Model resource:
- Specifies which LLM model to use (e.g., gpt-4o, claude-sonnet-4)
- References a ModelProvider for connection configuration
- Defines model parameters like temperature, max tokens, etc.
- Can include provider-specific advanced parameters

## Basic Structure

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Model
metadata:
  name: my-model
spec:
  model: "gpt-4o"
  
  providerRef:
    name: openai-provider
    namespace: shared-resources  # Optional
  
  parameters:
    temperature: "0.7"
    maxTokens: 4096
```

## Model Names by Provider

### OpenAI Models

```yaml
spec:
  model: "gpt-4o"              # GPT-4 Omni (recommended)
  # model: "gpt-4o-mini"       # Smaller, faster GPT-4 Omni
  # model: "gpt-4-turbo"       # GPT-4 Turbo
  # model: "gpt-3.5-turbo"     # GPT-3.5
  # model: "o1"                # OpenAI o1 reasoning model
  # model: "o3-mini"           # OpenAI o3-mini reasoning model
```

### Anthropic Models

```yaml
spec:
  model: "claude-sonnet-4-20250514"      # Claude Sonnet 4 (latest)
  # model: "claude-opus-4-20250514"      # Claude Opus 4
  # model: "claude-3-5-sonnet-20241022"  # Claude 3.5 Sonnet
  # model: "claude-3-5-haiku-20241022"   # Claude 3.5 Haiku
```

### Google Models

```yaml
spec:
  model: "gemini-2.0-flash-exp"     # Gemini 2.0 Flash
  # model: "gemini-exp-1206"        # Gemini Experimental
  # model: "gemini-1.5-pro"         # Gemini 1.5 Pro
  # model: "gemini-1.5-flash"       # Gemini 1.5 Flash
```

### AWS Bedrock Models

```yaml
spec:
  model: "anthropic.claude-3-5-sonnet-20241022-v2:0"
  # model: "anthropic.claude-3-5-haiku-20241022-v1:0"
  # model: "amazon.nova-pro-v1:0"
  # model: "amazon.nova-lite-v1:0"
```

## Common Parameters

All providers support these base parameters:

```yaml
spec:
  parameters:
    # Temperature: controls randomness (0.0 = deterministic, 2.0 = very random)
    temperature: "0.7"
    
    # Max tokens to generate in response
    maxTokens: 4096
    
    # Top-p sampling (0.0 to 1.0)
    topP: "0.9"
    
    # Top-k sampling (limits token choices)
    topK: 40
    
    # Presence penalty (-2.0 to 2.0)
    presencePenalty: "0.0"
    
    # Frequency penalty (-2.0 to 2.0)
    frequencyPenalty: "0.0"
    
    # Response timeout in seconds
    timeOut: 60
    
    # Enable parallel tool calls
    parallelToolCalls: true
    
    # Stop sequences
    stopSequences:
      - "\n\nUser:"
      - "<|end|>"
    
    # Seed for deterministic generation (where supported)
    seed: 42
```

## Provider-Specific Parameters

### OpenAI Parameters

```yaml
spec:
  model: "o1"
  providerRef:
    name: openai-provider
  
  parameters:
    # Base parameters
    temperature: "1.0"
    maxTokens: 16000
    
    # OpenAI-specific
    openai:
      # Reasoning effort for o1/o3 models
      reasoningEffort: "high"  # none, minimal, low, medium, high, xhigh
      
      # Log probabilities
      logProbs: true
      topLogProbs: 5
      
      # User identifier
      user: "user-123"
      
      # Service tier for requests
      serviceTier: "auto"  # auto, default, flex, priority
      
      # Prompt caching
      promptCacheKey: "my-cache-key"
      promptRetention: "ephemeral"
      
      # Reasoning output control
      reasoningGenerateSummary: "concise"  # detailed, concise
      sendReasoningIDs: true
      
      # Response handling
      truncation: "auto"  # disabled, auto
      textVerbosity: "medium"  # low, medium, high
      
      # Continue from previous response
      previousResponseID: "auto"
      
      # Include additional outputs
      includeCodeExecutionOutputs: true
      includeWebSearchSources: false
      includeFileSearchResults: false
      includeRawAnnotations: false
```

### Anthropic Parameters

```yaml
spec:
  model: "claude-sonnet-4-20250514"
  providerRef:
    name: anthropic-provider
  
  parameters:
    temperature: "0.7"
    maxTokens: 8192
    
    # Anthropic-specific
    anthropic:
      # User ID for tracking
      metadataUserID: "user-123"
      
      # Extended thinking configuration
      thinking:
        type: "enabled"  # enabled, disabled
        budgetTokens: 4096  # Must be >= 1024
      
      # Prompt caching
      cacheToolDefinitions: "5m"  # true, "5m", or "1h"
      cacheInstructions: "5m"
      cacheMessages: "5m"
      
      # Multi-turn conversation container
      container:
        id: "container_xxx"
        disabled: false
```

### Google Parameters

```yaml
spec:
  model: "gemini-2.0-flash-exp"
  providerRef:
    name: google-provider
  
  parameters:
    temperature: "0.7"
    maxTokens: 8192
    topK: 40
    
    # Google-specific
    google:
      # Thinking configuration
      thinkingConfig:
        includeThoughts: true
        thinkingBudget: -1  # -1 = automatic, 0 = disabled, >0 = token budget
        thinkingLevel: "medium"  # unspecified, minimal, low, medium, high
      
      # Safety settings
      safetySettings:
        - category: "HARM_CATEGORY_HARASSMENT"
          threshold: "BLOCK_MEDIUM_AND_ABOVE"
          method: "PROBABILITY"
        - category: "HARM_CATEGORY_HATE_SPEECH"
          threshold: "BLOCK_MEDIUM_AND_ABOVE"
      
      # Vertex AI specific
      labels:
        team: "ml-ops"
        environment: "production"
      
      # Media resolution
      videoResolution: "high"  # unspecified, low, medium, high
      
      # Cached content reference
      cachedContent: "cached-content-name"
```

### AWS Bedrock Parameters

```yaml
spec:
  model: "anthropic.claude-3-5-sonnet-20241022-v2:0"
  providerRef:
    name: bedrock-provider
  
  parameters:
    temperature: "0.7"
    maxTokens: 4096
    
    # Bedrock-specific
    bedrock:
      # Content moderation guardrails
      guardrailConfig:
        guardrailIdentifier: "guardrail-id"
        guardrailVersion: "1"
        trace: "enabled"  # disabled, enabled, enabled_full
      
      # Performance optimization
      performanceConfiguration:
        latency: "optimized"  # optimized, standard
      
      # Request metadata
      requestMetadata:
        application: "my-app"
        team: "ml-team"
      
      # Extract additional response fields
      additionalModelResponseFieldsPaths:
        - "$.custom_field"
      
      # Prompt template variables
      promptVariables:
        user_name: "John"
        context: "customer_support"
      
      # Caching
      cacheToolDefinitions: true
      cacheInstructions: true
      cacheMessages: true
      
      # Service tier
      serviceTier:
        type: "default"  # default, flex, priority, reserved
```

## Examples

### Simple Model Configuration

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: ModelProvider
metadata:
  name: openai-provider
spec:
  apiKeySecretRef:
    name: openai-credentials
    key: api-key
  openai: {}
---
apiVersion: agent.flokoa.ai/v1alpha1
kind: Model
metadata:
  name: gpt-4o-default
spec:
  model: "gpt-4o"
  providerRef:
    name: openai-provider
```

### Model with Custom Parameters

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Model
metadata:
  name: gpt-4o-creative
spec:
  model: "gpt-4o"
  providerRef:
    name: openai-provider
  
  parameters:
    temperature: "1.2"        # Higher for more creative responses
    maxTokens: 8192
    topP: "0.95"
    presencePenalty: "0.6"    # Encourage diverse vocabulary
    frequencyPenalty: "0.3"   # Reduce repetition
```

### Model for Code Generation

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Model
metadata:
  name: gpt-4o-code
spec:
  model: "gpt-4o"
  providerRef:
    name: openai-provider
  
  parameters:
    temperature: "0.2"        # Lower for more deterministic code
    maxTokens: 16384          # More tokens for longer code
    topP: "0.1"
    stopSequences:
      - "```\n\n"             # Stop after code block
```

### Reasoning Model Configuration

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Model
metadata:
  name: o1-reasoning
spec:
  model: "o1"
  providerRef:
    name: openai-provider
  
  parameters:
    maxTokens: 32768
    
    openai:
      reasoningEffort: "high"
      sendReasoningIDs: true
      reasoningSummary: "detailed"
```

### Claude with Extended Thinking

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Model
metadata:
  name: claude-thinking
spec:
  model: "claude-sonnet-4-20250514"
  providerRef:
    name: anthropic-provider
  
  parameters:
    temperature: "0.7"
    maxTokens: 8192
    
    anthropic:
      thinking:
        type: "enabled"
        budgetTokens: 4096
      
      # Enable caching for cost optimization
      cacheInstructions: "5m"
      cacheToolDefinitions: "5m"
```

### Multi-Model Setup

```yaml
# Fast model for simple tasks
apiVersion: agent.flokoa.ai/v1alpha1
kind: Model
metadata:
  name: gpt-4o-mini-fast
spec:
  model: "gpt-4o-mini"
  providerRef:
    name: openai-provider
  parameters:
    temperature: "0.3"
    maxTokens: 2048
    timeOut: 30
---
# Powerful model for complex tasks
apiVersion: agent.flokoa.ai/v1alpha1
kind: Model
metadata:
  name: gpt-4o-powerful
spec:
  model: "gpt-4o"
  providerRef:
    name: openai-provider
  parameters:
    temperature: "0.7"
    maxTokens: 16384
    timeOut: 120
---
# Agent using the fast model by default
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: my-agent
spec:
  model:
    name: gpt-4o-mini-fast
  # ... rest of agent config
```

### Cross-Namespace Model

```yaml
# In shared-models namespace
apiVersion: agent.flokoa.ai/v1alpha1
kind: Model
metadata:
  name: shared-gpt-4o
  namespace: shared-models
spec:
  model: "gpt-4o"
  providerRef:
    name: openai-provider
    namespace: shared-resources
  parameters:
    temperature: "0.7"
    maxTokens: 8192
---
# Agent in different namespace referencing shared model
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: my-agent
  namespace: my-app
spec:
  model:
    name: shared-gpt-4o
    namespace: shared-models
  # ... rest of agent config
```

## Status Fields

```yaml
status:
  ready: true
  
  resolvedProvider:
    provider: openai
    namespace: default
    name: openai-provider
  
  conditions:
    - type: Ready
      status: "True"
      lastTransitionTime: "2026-01-15T10:30:00Z"
      reason: ProviderFound
      message: "Model is configured and ready"
  
  observedGeneration: 1
```

## Operations

### Viewing Models

```bash
# List all models
kubectl get models

# Get detailed information
kubectl describe model gpt-4o-default

# Check readiness
kubectl get model gpt-4o-default -o jsonpath='{.status.ready}'
```

### Updating Model Parameters

```bash
# Update temperature
kubectl patch model gpt-4o-default --type='json' \
  -p='[{"op": "replace", "path": "/spec/parameters/temperature", "value": "0.8"}]'

# Update max tokens
kubectl patch model gpt-4o-default --type='json' \
  -p='[{"op": "replace", "path": "/spec/parameters/maxTokens", "value": 8192}]'
```

### Testing Model Configuration

Create a simple agent to test the model:

```yaml
apiVersion: agent.flokoa.ai/v1alpha1
kind: Agent
metadata:
  name: model-test-agent
spec:
  model:
    name: gpt-4o-default
  runtime:
    type: standard
    spec:
      container:
        name: agent
        image: ghcr.io/example/test-agent:latest
        ports:
        - containerPort: 8080
```

## Best Practices

1. **Name models descriptively** - Include provider and use case (e.g., `gpt-4o-code`, `claude-creative`)
2. **Use shared models** - Create models in a shared namespace for team-wide use
3. **Start with defaults** - Only customize parameters when needed
4. **Test configurations** - Create test agents to validate model settings
5. **Document parameters** - Use labels/annotations to explain parameter choices
6. **Version control** - Keep model configs in Git alongside application code
7. **Monitor costs** - Track token usage, especially with high maxTokens
8. **Use appropriate models** - Match model size to task complexity
9. **Set timeouts** - Configure reasonable timeouts for your use case
10. **Consider caching** - Enable provider caching features for cost savings

## Parameter Guidelines

### Temperature
- `0.0-0.3`: Deterministic tasks (code, math, classification)
- `0.4-0.7`: Balanced responses (general Q&A, analysis)
- `0.8-1.2`: Creative tasks (writing, brainstorming)
- `1.3-2.0`: Highly creative/exploratory outputs

### Max Tokens
- Short responses: 512-2048
- Medium responses: 2048-8192
- Long-form content: 8192-16384
- Code generation: 16384-32768

### Top P
- `0.1-0.3`: Very focused responses
- `0.4-0.7`: Balanced diversity
- `0.8-1.0`: Maximum diversity

## Troubleshooting

### Model Not Ready

```bash
# Check model status
kubectl describe model <name>

# Common issues:
# - ModelProvider not found or not ready
# - Invalid model name for provider
# - Provider authentication issues
```

### Rate Limiting

If you hit rate limits:
1. Increase timeout: `timeOut: 120`
2. Add retry logic in your agent code
3. Use multiple API keys/providers
4. Implement exponential backoff

### High Costs

Monitor and optimize:
```yaml
parameters:
  maxTokens: 4096  # Set reasonable limits
  openai:
    promptCacheKey: "my-prompt"  # Use caching
```

### Invalid Parameters

- Check parameter ranges in provider documentation
- Verify parameter names match exactly (case-sensitive)
- Some parameters only work with specific models
- Validate JSON schema for complex parameters

### Testing Parameter Changes

```bash
# Create test model variant
kubectl apply -f - <<EOF
apiVersion: agent.flokoa.ai/v1alpha1
kind: Model
metadata:
  name: gpt-4o-test
spec:
  model: "gpt-4o"
  providerRef:
    name: openai-provider
  parameters:
    temperature: "0.9"  # Test new temperature
    maxTokens: 4096
EOF

# Compare results with original model
# If satisfied, update original or promote test config
```
