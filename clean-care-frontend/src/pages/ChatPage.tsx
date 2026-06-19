import { useCallback, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { createConversation, getMessages } from '../api/conversations';
import { getTrace } from '../api/traces';
import useSSEStream from '../hooks/useSSEStream';
import usePipeline, { type PipelineAction } from '../hooks/usePipeline';
import useKeyboardShortcuts from '../hooks/useKeyboardShortcuts';
import { useAppStore } from '../store/appStore';
import type { Message } from '../types/conversation';
import type { StatusEvent, EvidenceEvent, DoneEvent, SSEErrorEvent } from '../types/sse';
import { addConversationToStorage } from '../components/layout/Sidebar';
import ChatArea from '../components/chat/ChatArea';
import ChatInput from '../components/chat/ChatInput';
import StatusBar from '../components/chat/StatusBar';
import ConnectionStatus from '../components/chat/ConnectionStatus';
import PipelinePanel from '../components/pipeline/PipelinePanel';
import ErrorMessage from '../components/ui/ErrorMessage';
import { ApiError } from '../types/api';

export default function ChatPage() {
  const { conversationId } = useParams();
  const navigate = useNavigate();

  const {
    messages,
    streamingContent,
    traceId,
    error,
    isLoadingMessages,
    messagesError,
    pipelineDrawerOpen,
    setMessages,
    setStreamingContent,
    setTraceId,
    setError,
    addMessage,
    setIsLoadingMessages,
    setMessagesError,
    resetChatState,
    toggleSidebar,
    togglePipelineDrawer,
    connectionState,
    reconnectCount,
  } = useAppStore();

  const { pipeline, dispatch: pipelineDispatch } = usePipeline();
  const { startStream, abort, isStreaming, connectionState: sseConnectionState, reconnectCount: sseReconnectCount } = useSSEStream();

  // Keyboard shortcuts
  useKeyboardShortcuts({
    onNewConversation: () => navigate('/chat'),
    onToggleSidebar: toggleSidebar,
    onTogglePipeline: togglePipelineDrawer,
  });

  // Sync SSE state to store
  useEffect(() => {
    useAppStore.setState({
      isStreaming,
      connectionState: sseConnectionState,
      reconnectCount: sseReconnectCount,
    });
  }, [isStreaming, sseConnectionState, sseReconnectCount]);

  // Load messages when conversationId changes
  useEffect(() => {
    if (!conversationId) {
      resetChatState();
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
          setMessagesError(err instanceof ApiError ? err.message : '加载消息失败');
        }
      })
      .finally(() => {
        if (!cancelled) setIsLoadingMessages(false);
      });

    return () => {
      cancelled = true;
    };
  }, [conversationId, pipelineDispatch, resetChatState, setIsLoadingMessages, setMessages, setMessagesError]);

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
          useAppStore.getState().addConversation(conv);
          navigate(`/chat/${convId}`, { replace: true });
        } catch (err) {
          setError(
            err instanceof ApiError ? err.message : '创建对话失败'
          );
          return;
        }
      }

      // Add user message optimistically
      const userMsg: Message = {
        id: `local_${Date.now()}`,
        role: 'user',
        content,
        created_at: new Date().toISOString(),
      };
      addMessage(userMsg);

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
          const aiMsg: Message = {
            id: data.message_id,
            role: 'assistant',
            content: assistantContent,
            trace_id: data.trace_id,
            created_at: new Date().toISOString(),
          };
          addMessage(aiMsg);
          setStreamingContent('');
          setTraceId(data.trace_id);

          pipelineDispatch({ type: 'MARK_COMPLETE' });

          if (data.trace_id) {
            try {
              const trace = await getTrace(data.trace_id);
              pipelineDispatch({ type: 'ENRICH_TRACE', trace });
            } catch {
              // Non-fatal
            }
          }
        },

        onError: (err: SSEErrorEvent) => {
          setError(err.message || err.code);
          pipelineDispatch({ type: 'MARK_FAILED', error: err.code });
        },
      });
    },
    [conversationId, navigate, startStream, pipelineDispatch, setError, setStreamingContent, setTraceId, addMessage]
  );

  const handleAbort = useCallback(() => {
    abort();
    if (streamingContent) {
      const aiMsg: Message = {
        id: `aborted_${Date.now()}`,
        role: 'assistant',
        content: streamingContent + '\n\n*[已中断]*',
        created_at: new Date().toISOString(),
      };
      addMessage(aiMsg);
      setStreamingContent('');
    }
  }, [abort, streamingContent, addMessage, setStreamingContent]);

  const handleSuggestionClick = (text: string) => {
    handleSend(text);
  };

  const handleHandoff = useCallback(() => {
    handleSend('请转人工客服接管当前问题');
  }, [handleSend]);

  return (
    <div className="chat-page">
      {/* Top bar: pipeline status + connection */}
      <div className="chat-topbar">
        <div className="chat-topbar__left">
          <button
            className="chat-topbar__menu-btn"
            onClick={toggleSidebar}
            title="切换侧边栏"
          >
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M3 12h18M3 6h18M3 18h18" />
            </svg>
          </button>
          <StatusBar pipeline={pipeline} isStreaming={isStreaming} />
          <ConnectionStatus state={connectionState} reconnectCount={reconnectCount} />
        </div>
        <div className="chat-topbar__right">
          <button
            className={`chat-topbar__pipeline-btn ${pipelineDrawerOpen ? 'chat-topbar__pipeline-btn--active' : ''}`}
            onClick={togglePipelineDrawer}
            title="Agent Pipeline"
          >
            🔍 Pipeline
          </button>
        </div>
      </div>

      {error && (
        <div className="chat-error-bar">
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
                setMessagesError(err instanceof ApiError ? err.message : '加载失败')
              )
              .finally(() => setIsLoadingMessages(false));
          }
        }}
        onSuggestionClick={handleSuggestionClick}
      />

      <div className="chat-input-container">
        <ChatInput
          onSend={handleSend}
          onAbort={handleAbort}
          onHandoff={handleHandoff}
          isStreaming={isStreaming}
        />
      </div>

      {/* Pipeline drawer overlay */}
      {pipelineDrawerOpen && (
        <div className="pipeline-drawer-overlay" onClick={togglePipelineDrawer} />
      )}
      <div className={`pipeline-drawer ${pipelineDrawerOpen ? 'pipeline-drawer--open' : ''}`}>
        <PipelinePanel steps={pipeline.steps} traceId={traceId || pipeline.traceId} />
      </div>
    </div>
  );
}
