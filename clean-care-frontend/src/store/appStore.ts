import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import type { Conversation, Message } from '../types/conversation';
import type { ConnectionState } from '../hooks/useSSEStream';

export type ThemeMode = 'light' | 'dark' | 'system';

interface AppState {
  // Conversations
  conversations: Conversation[];
  currentConversationId: string | null;

  // Messages
  messages: Message[];
  streamingContent: string;
  isLoadingMessages: boolean;
  messagesError: string | null;

  // SSE State
  isStreaming: boolean;
  connectionState: ConnectionState;
  reconnectCount: number;

  // Trace
  traceId: string | null;

  // UI State
  error: string | null;
  theme: ThemeMode;
  sidebarOpen: boolean;
  pipelineDrawerOpen: boolean;

  // Actions
  setConversations: (conversations: Conversation[]) => void;
  addConversation: (conversation: Conversation) => void;
  updateConversation: (id: string, updates: Partial<Conversation>) => void;
  removeConversation: (id: string) => void;
  setCurrentConversationId: (id: string | null) => void;

  setMessages: (messages: Message[]) => void;
  addMessage: (message: Message) => void;
  setStreamingContent: (content: string) => void;
  setIsLoadingMessages: (loading: boolean) => void;
  setMessagesError: (error: string | null) => void;

  setIsStreaming: (streaming: boolean) => void;
  setConnectionState: (state: ConnectionState) => void;
  setReconnectCount: (count: number) => void;

  setTraceId: (id: string | null) => void;
  setError: (error: string | null) => void;
  setTheme: (theme: ThemeMode) => void;
  setSidebarOpen: (open: boolean) => void;
  setPipelineDrawerOpen: (open: boolean) => void;
  toggleSidebar: () => void;
  togglePipelineDrawer: () => void;

  // Reset state
  resetChatState: () => void;
  resetAll: () => void;
}

function defaultSidebarOpen(): boolean {
  return typeof window === 'undefined' ? true : window.innerWidth > 900;
}

const initialState = {
  conversations: [],
  currentConversationId: null,
  messages: [],
  streamingContent: '',
  isLoadingMessages: false,
  messagesError: null,
  isStreaming: false,
  connectionState: 'idle' as ConnectionState,
  reconnectCount: 0,
  traceId: null,
  error: null,
  theme: 'system' as ThemeMode,
  sidebarOpen: defaultSidebarOpen(),
  pipelineDrawerOpen: false,
};

export const useAppStore = create<AppState>()(
  persist(
    (set) => ({
      ...initialState,

      // Conversation actions
      setConversations: (conversations) => set({ conversations }),

      addConversation: (conversation) =>
        set((state) => ({
          conversations: [conversation, ...state.conversations],
        })),

      updateConversation: (id, updates) =>
        set((state) => ({
          conversations: state.conversations.map((conv) =>
            conv.conversation_id === id ? { ...conv, ...updates } : conv
          ),
        })),

      removeConversation: (id) =>
        set((state) => ({
          conversations: state.conversations.filter(
            (conv) => conv.conversation_id !== id
          ),
          currentConversationId:
            state.currentConversationId === id ? null : state.currentConversationId,
        })),

      setCurrentConversationId: (id) => set({ currentConversationId: id }),

      // Message actions
      setMessages: (messages) => set({ messages }),

      addMessage: (message) =>
        set((state) => ({
          messages: [...state.messages, message],
        })),

      setStreamingContent: (streamingContent) => set({ streamingContent }),

      setIsLoadingMessages: (isLoadingMessages) => set({ isLoadingMessages }),

      setMessagesError: (messagesError) => set({ messagesError }),

      // SSE actions
      setIsStreaming: (isStreaming) => set({ isStreaming }),

      setConnectionState: (connectionState) => set({ connectionState }),

      setReconnectCount: (reconnectCount) => set({ reconnectCount }),

      // Other actions
      setTraceId: (traceId) => set({ traceId }),

      setError: (error) => set({ error }),

      setTheme: (theme) => {
        set({ theme });
        // Apply theme to document
        const root = document.documentElement;
        if (theme === 'dark') {
          root.classList.add('dark');
          root.classList.remove('light');
        } else if (theme === 'light') {
          root.classList.add('light');
          root.classList.remove('dark');
        } else {
          // System theme
          const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
          root.classList.toggle('dark', prefersDark);
          root.classList.toggle('light', !prefersDark);
        }
      },

      setSidebarOpen: (sidebarOpen) => set({ sidebarOpen }),

      setPipelineDrawerOpen: (pipelineDrawerOpen) => set({ pipelineDrawerOpen }),

      toggleSidebar: () => set((state) => ({ sidebarOpen: !state.sidebarOpen })),

      togglePipelineDrawer: () =>
        set((state) => ({ pipelineDrawerOpen: !state.pipelineDrawerOpen })),

      // Reset actions
      resetChatState: () =>
        set({
          messages: [],
          streamingContent: '',
          isLoadingMessages: false,
          messagesError: null,
          isStreaming: false,
          connectionState: 'idle',
          reconnectCount: 0,
          traceId: null,
          error: null,
        }),

      resetAll: () => set(initialState),
    }),
    {
      name: 'cleancare-app-store',
      partialize: (state) => ({
        conversations: state.conversations,
        theme: state.theme,
      }),
      merge: (persistedState, currentState) => {
        // Ensure theme always has a valid value
        const persisted = persistedState as Partial<AppState>;
        return {
          ...currentState,
          ...persisted,
          theme: persisted?.theme ?? 'system',
          sidebarOpen: defaultSidebarOpen(),
        };
      },
    }
  )
);
