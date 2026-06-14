import { useCallback, useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { createConversation, getMessages } from '../api/conversations';
import { getTrace } from '../api/traces';
import useSSEStream from '../hooks/useSSEStream';
import usePipeline, { type PipelineAction } from '../hooks/usePipeline';
import type { Message } from '../types/conversation';
import type { StatusEvent, EvidenceEvent, DoneEvent, SSEErrorEvent } from '../types/sse';
import { addConversationToStorage } from '../components/chat/ConversationList';
import ChatArea from '../components/chat/ChatArea';
import ChatInput from '../components/chat/ChatInput';
import StatusBar from '../components/chat/StatusBar';
import PipelinePanel from '../components/pipeline/PipelinePanel';
import ErrorMessage from '../components/ui/ErrorMessage';
import { ApiError } from '../types/api';

export default function ChatPage() {
  const { conversationId } = useParams();
  const navigate = useNavigate();

  const [messages, setMessages] = useState<Message[]>([]);
  const [isLoadingMessages, setIsLoadingMessages] = useState(false);
  const [messagesError, setMessagesError] = useState<string | null>(null);
  const [streamingContent, setStreamingContent] = useState('');
  const [traceId, setTraceId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const { pipeline, dispatch: pipelineDispatch } = usePipeline();
  const { startStream, abort, isStreaming } = useSSEStream();

  // Load messages when conversationId changes
  useEffect(() => {
    if (!conversationId) {
      setMessages([]);
      setStreamingContent('');
      setTraceId(null);
      pipelineDispatch({ type: 'RESET' });
      return;
    }

    let cancelled = false;
    setIsLoadingMessages(true);
    setMessagesError(null);

    getMessages(conversationId, 20)
      .then((data) => {
        if (!cancelled) {
          setMessages(data.items);
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setMessagesError(err instanceof ApiError ? err.message : 'Failed to load messages');
        }
      })
      .finally(() => {
        if (!cancelled) setIsLoadingMessages(false);
      });

    return () => {
      cancelled = true;
    };
  }, [conversationId, pipelineDispatch]);

  // Abort SSE on unmount
  useEffect(() => {
    return () => {
      abort();
    };
  }, [abort]);

  const handleSend = useCallback(
    async (content: string) => {
      setError(null);
      setStreamingContent('');
      pipelineDispatch({ type: 'RESET' });

      let convId = conversationId;

      // Create conversation if needed
      if (!convId) {
        try {
          const conv = await createConversation(
            content.slice(0, 30) + (content.length > 30 ? '...' : '')
          );
          convId = conv.conversation_id;
          addConversationToStorage(conv);
          navigate(`/chat/${convId}`, { replace: true });
        } catch (err) {
          setError(
            err instanceof ApiError ? err.message : 'Failed to create conversation'
          );
          return;
        }
      }

      // Add user message optimistically
      const userMsg: Message = { id: `local_${Date.now()}`, role: 'user', content };
      setMessages((prev) => [...prev, userMsg]);

      let assistantContent = '';

      startStream(convId, content, {
        onStatus: (data: StatusEvent) => {
          pipelineDispatch({
            type: 'ADVANCE_STAGE',
            stage: data.stage,
            detail: data,
          } as PipelineAction);
          if (data.trace_id) setTraceId(data.trace_id);
        },

        onEvidence: (data: EvidenceEvent) => {
          pipelineDispatch({
            type: 'APPEND_EVIDENCE',
            evidence: data,
          } as PipelineAction);
        },

        onDelta: (content: string) => {
          assistantContent += content;
          setStreamingContent(assistantContent);
        },

        onDone: async (data: DoneEvent) => {
          // Finalize the AI message
          const aiMsg: Message = {
            id: data.message_id,
            role: 'assistant',
            content: assistantContent,
            trace_id: data.trace_id,
          };
          setMessages((prev) => [...prev, aiMsg]);
          setStreamingContent('');
          setTraceId(data.trace_id);

          pipelineDispatch({ type: 'MARK_COMPLETE' });

          // Fetch full trace to enrich pipeline
          if (data.trace_id) {
            try {
              const trace = await getTrace(data.trace_id);
              pipelineDispatch({ type: 'ENRICH_TRACE', trace });
            } catch {
              // Trace fetch failure is non-fatal — pipeline already shows SSE data
            }
          }
        },

        onError: (err: SSEErrorEvent) => {
          setError(err.message || err.code);
          pipelineDispatch({ type: 'MARK_FAILED', error: err.code });
        },
      });
    },
    [conversationId, navigate, startStream, pipelineDispatch]
  );

  const handleAbort = useCallback(() => {
    abort();
    if (streamingContent) {
      const aiMsg: Message = {
        id: `aborted_${Date.now()}`,
        role: 'assistant',
        content: streamingContent + '\n\n*[Interrupted]*',
      };
      setMessages((prev) => [...prev, aiMsg]);
      setStreamingContent('');
    }
  }, [abort, streamingContent]);

  return (
    <div className="chat-page">
      <div className="chat-area">
        <StatusBar pipeline={pipeline} isStreaming={isStreaming} />
        {error && (
          <div style={{ padding: '8px 24px 0' }}>
            <ErrorMessage message={error} onRetry={() => setError(null)} />
          </div>
        )}
        <ChatArea
          messages={messages}
          streamingContent={streamingContent}
          isLoading={isLoadingMessages}
          error={messagesError}
          onRetryLoad={() => {
            if (conversationId) {
              setMessagesError(null);
              setIsLoadingMessages(true);
              getMessages(conversationId, 20)
                .then((data) => setMessages(data.items))
                .catch((err) =>
                  setMessagesError(err instanceof ApiError ? err.message : 'Failed')
                )
                .finally(() => setIsLoadingMessages(false));
            }
          }}
        />
        <div className="chat-input-container">
          <ChatInput
            onSend={handleSend}
            onAbort={handleAbort}
            isStreaming={isStreaming}
          />
        </div>
      </div>
      <PipelinePanel steps={pipeline.steps} traceId={traceId || pipeline.traceId} />
    </div>
  );
}
