package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	documentai "cloud.google.com/go/documentai/apiv1"
	documentaipb "cloud.google.com/go/documentai/apiv1/documentaipb"
	lroauto "cloud.google.com/go/longrunning/autogen"
	longrunningpb "cloud.google.com/go/longrunning/autogen/longrunningpb"
	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	executor "github.com/argoproj/argo-workflows/v3/pkg/plugins/executor"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/encoding/protojson"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/danielnyari/flokoa/internal/telemetry"
)

// Plugin implements the Argo Workflows executor plugin for GCP Document AI.
type Plugin struct {
	// tasks stores in-progress LRO state keyed by workflow UID + template name.
	tasks *StateStore
}

// New creates a new GCP Document AI executor plugin with persistent state storage.
func New(k8sClient client.Client, namespace string) *Plugin {
	return &Plugin{
		tasks: NewStateStore(k8sClient, namespace),
	}
}

// taskKey generates a unique key for tracking tasks.
func taskKey(workflowUID, templateName string) string {
	return fmt.Sprintf("%s/%s", workflowUID, templateName)
}

// ExecuteTemplate handles the execution of a GCP Document AI plugin template.
func (p *Plugin) ExecuteTemplate(ctx context.Context, args executor.ExecuteTemplateArgs) (*executor.ExecuteTemplateReply, error) {
	spec, err := parseGCPDocAISpec(args.Template)
	if err != nil {
		return failedReply(fmt.Sprintf("failed to parse GCP DocAI spec: %v", err)), nil
	}

	// Restore the distributed trace context from the traceparent.
	if spec.Traceparent != "" {
		ctx = telemetry.ContextFromTraceparent(ctx, spec.Traceparent)
	}

	ctx, span := telemetry.Tracer("flokoa.gcpdocai-plugin").Start(ctx, "gcpdocai.execute_template",
		trace.WithAttributes(
			attribute.String("gcpdocai.processor", spec.ProcessorName),
			attribute.String("gcpdocai.location", spec.Location),
			attribute.String("gcpdocai.template", args.Template.Name),
		),
	)
	defer span.End()

	key := taskKey(args.Workflow.ObjectMeta.UID, args.Template.Name)

	// Check if this is a requeue (we have an in-progress LRO)
	if progress, ok := p.tasks.Load(key); ok {
		return p.pollOperation(ctx, key, progress)
	}

	// New task: submit batch process
	return p.submitBatchProcess(ctx, key, spec)
}

// submitBatchProcess submits a new BatchProcessDocuments request to Document AI.
func (p *Plugin) submitBatchProcess(ctx context.Context, key string, spec *GCPDocAISpec) (*executor.ExecuteTemplateReply, error) {
	ctx, span := telemetry.Tracer("flokoa.gcpdocai-plugin").Start(ctx, "gcpdocai.submit",
		trace.WithAttributes(
			attribute.String("gcpdocai.processor", spec.ProcessorName),
			attribute.String("gcpdocai.location", spec.Location),
		),
	)
	defer span.End()

	endpoint := fmt.Sprintf("%s-documentai.googleapis.com:443", spec.Location)

	docaiClient, err := documentai.NewDocumentProcessorClient(ctx,
		option.WithEndpoint(endpoint),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create Document AI client")
		return failedReply(fmt.Sprintf("failed to create Document AI client: %v", err)), nil
	}
	defer docaiClient.Close()

	// Build the BatchProcessRequest from the spec using the proto types directly.
	req := &documentaipb.BatchProcessRequest{
		Name:            spec.ProcessorName,
		SkipHumanReview: spec.SkipHumanReview,
	}

	// Build InputDocuments
	inputDocs := &documentaipb.BatchDocumentsInputConfig{}
	if len(spec.InputDocuments.GCSDocuments) > 0 {
		gcsDocsList := &documentaipb.GcsDocuments{
			Documents: make([]*documentaipb.GcsDocument, 0, len(spec.InputDocuments.GCSDocuments)),
		}
		for _, doc := range spec.InputDocuments.GCSDocuments {
			gcsDocsList.Documents = append(gcsDocsList.Documents, &documentaipb.GcsDocument{
				GcsUri:   doc.GCSUri,
				MimeType: doc.MimeType,
			})
		}
		inputDocs.Source = &documentaipb.BatchDocumentsInputConfig_GcsDocuments{
			GcsDocuments: gcsDocsList,
		}
	} else if spec.InputDocuments.GCSPrefix != nil {
		inputDocs.Source = &documentaipb.BatchDocumentsInputConfig_GcsPrefix{
			GcsPrefix: &documentaipb.GcsPrefix{
				GcsUriPrefix: spec.InputDocuments.GCSPrefix.GCSUriPrefix,
			},
		}
	}
	req.InputDocuments = inputDocs

	// Build OutputConfig
	gcsOutputCfg := &documentaipb.DocumentOutputConfig_GcsOutputConfig{
		GcsUri:    spec.OutputConfig.GCSUri,
		FieldMask: nil,
	}
	if spec.OutputConfig.FieldMask != "" {
		gcsOutputCfg.FieldMask = &documentaipb.DocumentOutputConfig_GcsOutputConfig_ShardingConfig{}
		// FieldMask is set on the DocumentOutputConfig level, not here.
		// For simplicity, we pass it through GCS output config.
	}
	req.DocumentOutputConfig = &documentaipb.DocumentOutputConfig{
		Destination: &documentaipb.DocumentOutputConfig_GcsOutputConfig_{
			GcsOutputConfig: gcsOutputCfg,
		},
	}

	// Build ProcessOptions if present
	if spec.ProcessOptions != nil {
		processOpts := &documentaipb.ProcessOptions{}
		if spec.ProcessOptions.LayoutConfig != nil {
			layoutCfg := &documentaipb.ProcessOptions_LayoutConfig{}
			if spec.ProcessOptions.LayoutConfig.ChunkingConfig != nil {
				layoutCfg.ChunkingConfig = &documentaipb.ProcessOptions_LayoutConfig_ChunkingConfig{
					ChunkSize:               int32(spec.ProcessOptions.LayoutConfig.ChunkingConfig.ChunkSize),
					IncludeAncestorHeadings: spec.ProcessOptions.LayoutConfig.ChunkingConfig.IncludeAncestorHeadings,
				}
			}
			processOpts.IndividualPageSelector = &documentaipb.ProcessOptions_LayoutConfig_{
				LayoutConfig: layoutCfg,
			}
		}
		req.ProcessOptions = processOpts
	}

	log.Printf("Submitting BatchProcessDocuments: processor=%s location=%s", spec.ProcessorName, spec.Location)

	op, err := docaiClient.BatchProcessDocuments(ctx, req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to submit batch process")
		return failedReply(fmt.Sprintf("failed to submit BatchProcessDocuments: %v", err)), nil
	}

	operationName := op.Name()
	span.SetAttributes(attribute.String("gcpdocai.operation", operationName))

	log.Printf("BatchProcessDocuments submitted: operation=%s", operationName)

	// Store progress state for subsequent polls
	timeout := spec.GetTimeout()
	progress := &ProgressState{
		OperationName: operationName,
		Location:      spec.Location,
		StartTime:     time.Now(),
		Timeout:       timeout,
	}
	p.tasks.Store(key, progress)

	return &executor.ExecuteTemplateReply{
		Node: &wfv1.NodeResult{
			Phase:   wfv1.NodeRunning,
			Message: fmt.Sprintf("Document AI batch process submitted, operation: %s", operationName),
		},
		Requeue: &metav1.Duration{Duration: DefaultPollInterval},
	}, nil
}

// pollOperation polls an existing LRO for completion.
func (p *Plugin) pollOperation(ctx context.Context, key string, progress *ProgressState) (*executor.ExecuteTemplateReply, error) {
	ctx, span := telemetry.Tracer("flokoa.gcpdocai-plugin").Start(ctx, "gcpdocai.poll",
		trace.WithAttributes(
			attribute.String("gcpdocai.operation", progress.OperationName),
			attribute.String("gcpdocai.location", progress.Location),
		),
	)
	defer span.End()

	// Check for timeout
	if progress.IsTimedOut() {
		p.tasks.Delete(key)
		span.SetStatus(codes.Error, "operation timed out")
		return failedReply(fmt.Sprintf("Document AI operation timed out after %v", progress.Timeout)), nil
	}

	endpoint := fmt.Sprintf("%s-documentai.googleapis.com:443", progress.Location)

	lroClient, err := lroauto.NewOperationsClient(ctx, option.WithEndpoint(endpoint))
	if err != nil {
		span.RecordError(err)
		return failedReply(fmt.Sprintf("failed to create LRO client: %v", err)), nil
	}
	defer lroClient.Close()

	op, err := lroClient.GetOperation(ctx, &longrunningpb.GetOperationRequest{
		Name: progress.OperationName,
	})
	if err != nil {
		log.Printf("LRO poll failed: operation=%s error=%v", progress.OperationName, err)
		span.RecordError(err)

		progress.PollErrors++
		if progress.PollErrors < MaxPollErrors {
			log.Printf("Transient poll error (%d/%d), requeueing: %v", progress.PollErrors, MaxPollErrors, err)
			return &executor.ExecuteTemplateReply{
				Node: &wfv1.NodeResult{
					Phase:   wfv1.NodeRunning,
					Message: fmt.Sprintf("Document AI poll error (attempt %d/%d): %v", progress.PollErrors, MaxPollErrors, err),
				},
				Requeue: &metav1.Duration{Duration: DefaultPollInterval},
			}, nil
		}
		p.tasks.Delete(key)
		return failedReply(fmt.Sprintf("failed to poll Document AI operation after %d attempts: %v", progress.PollErrors, err)), nil
	}

	// Reset error counter on successful poll
	progress.PollErrors = 0

	if !op.GetDone() {
		// Still running, requeue
		return &executor.ExecuteTemplateReply{
			Node: &wfv1.NodeResult{
				Phase:   wfv1.NodeRunning,
				Message: fmt.Sprintf("Document AI operation in progress: %s", progress.OperationName),
			},
			Requeue: &metav1.Duration{Duration: DefaultPollInterval},
		}, nil
	}

	// Operation is done
	p.tasks.Delete(key)

	if op.GetError() != nil {
		errStatus := op.GetError()
		span.SetStatus(codes.Error, errStatus.GetMessage())
		return failedReply(fmt.Sprintf("Document AI batch process failed: %s (code %d)", errStatus.GetMessage(), errStatus.GetCode())), nil
	}

	// Success: extract metadata from the response
	span.SetStatus(codes.Ok, "batch process completed")

	result := progress.OperationName
	artifact := "{}"

	// Try to extract BatchProcessMetadata from the operation metadata
	if op.GetMetadata() != nil {
		var metadata documentaipb.BatchProcessMetadata
		if err := protojson.Unmarshal(op.GetMetadata().GetValue(), &metadata); err == nil {
			metadataJSON, jsonErr := protojson.Marshal(&metadata)
			if jsonErr == nil {
				artifact = string(metadataJSON)
			}
		}
	}

	log.Printf("Document AI batch process completed: operation=%s", progress.OperationName)

	return &executor.ExecuteTemplateReply{
		Node: &wfv1.NodeResult{
			Phase:   wfv1.NodeSucceeded,
			Message: "Document AI batch process completed successfully",
			Outputs: &wfv1.Outputs{
				Parameters: []wfv1.Parameter{
					{
						Name:  "result",
						Value: wfv1.AnyStringPtr(result),
					},
					{
						Name:  "artifact",
						Value: wfv1.AnyStringPtr(artifact),
					},
				},
			},
		},
	}, nil
}

// parseGCPDocAISpec extracts the GCP Document AI spec from the template.
func parseGCPDocAISpec(template *wfv1.Template) (*GCPDocAISpec, error) {
	if template == nil || template.Plugin == nil {
		return nil, fmt.Errorf("template or plugin is nil")
	}

	var pluginData map[string]json.RawMessage
	if err := json.Unmarshal(template.Plugin.Value, &pluginData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal plugin data: %w", err)
	}

	docaiData, ok := pluginData["gcpdocai"]
	if !ok {
		return nil, fmt.Errorf("no 'gcpdocai' key found in plugin spec")
	}

	var spec GCPDocAISpec
	if err := json.Unmarshal(docaiData, &spec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal GCP DocAI spec: %w", err)
	}

	if spec.ProcessorName == "" {
		return nil, fmt.Errorf("processorName is required")
	}
	if spec.Location == "" {
		return nil, fmt.Errorf("location is required")
	}

	return &spec, nil
}

// failedReply creates a failed ExecuteTemplateReply with the given message.
func failedReply(message string) *executor.ExecuteTemplateReply {
	return &executor.ExecuteTemplateReply{
		Node: &wfv1.NodeResult{
			Phase:   wfv1.NodeFailed,
			Message: message,
		},
	}
}
