import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import {
  FixedSizeList as List,
  type FixedSizeList,
  type ListChildComponentProps,
  type ListOnItemsRenderedProps,
} from "react-window";
import {
  deleteMessage,
  getMessage,
  getMe,
  getRawMessage,
  listMessages,
  login,
  logout,
  sendMessage,
} from "./api";
import type { MessageDetail, MessageSummary, User } from "./types";

type Box = "inbox" | "sent";
type ViewMode = "html" | "text" | "raw";

const dateFormatter = new Intl.DateTimeFormat("en-US", {
  month: "short",
  day: "numeric",
  hour: "2-digit",
  minute: "2-digit",
});

const longDateFormatter = new Intl.DateTimeFormat("en-US", {
  weekday: "short",
  month: "short",
  day: "numeric",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit",
});

const emptyMessage: MessageDetail = {
  id: "",
  from: "",
  to: [],
  cc: [],
  bcc: [],
  subject: "",
  text: "",
  html: "",
  createdAt: "",
  rawSize: 0,
  attachments: [],
};

const pageSize = 10;
const rowHeight = 108;

export default function App() {
  const [user, setUser] = useState<User | null>(null);
  const [box, setBox] = useState<Box>("inbox");
  const [messages, setMessages] = useState<MessageSummary[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [selectedMessage, setSelectedMessage] = useState<MessageDetail>(emptyMessage);
  const [detailTab, setDetailTab] = useState<ViewMode>("html");
  const [rawContent, setRawContent] = useState<string>("");
  const [searchInput, setSearchInput] = useState("");
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [detailError, setDetailError] = useState<string | null>(null);
  const [cursor, setCursor] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState(true);
  const [composeOpen, setComposeOpen] = useState(false);
  const [switchOpen, setSwitchOpen] = useState(false);
  const listViewportRef = useRef<HTMLDivElement | null>(null);
  const [listSize, setListSize] = useState({ height: 0, width: 0 });
  const listRef = useRef<FixedSizeList>(null);

  const loadMessages = useCallback(
    async (mode: "reset" | "append") => {
      if (!user) {
        return;
      }
      if (mode === "reset") {
        setLoading(true);
        setError(null);
        setCursor(null);
        setHasMore(true);
      } else {
        if (!hasMore || loadingMore || loading) {
          return;
        }
        setLoadingMore(true);
      }
      try {
        const response = await listMessages(
          box,
          search,
          mode === "append" ? cursor : null,
          pageSize
        );
        setMessages((current) => {
          const merged = mode === "append" ? [...current, ...response.messages] : response.messages;
          setSelectedId((selected) =>
            selected && merged.some((item) => item.id === selected) ? selected : null
          );
          return merged;
        });
        setCursor(response.nextCursor || null);
        setHasMore(response.hasMore);
        if (mode === "reset") {
          listRef.current?.scrollTo(0);
        }
        setError(null);
      } catch (err) {
        setError(err instanceof Error ? err.message : "Unable to load messages");
      } finally {
        if (mode === "reset") {
          setLoading(false);
        } else {
          setLoadingMore(false);
        }
      }
    },
    [box, cursor, hasMore, loading, loadingMore, search, user]
  );

  useEffect(() => {
    getMe().then(setUser).catch(() => setUser(null));
  }, []);

  useEffect(() => {
    const timer = window.setTimeout(() => setSearch(searchInput.trim()), 300);
    return () => window.clearTimeout(timer);
  }, [searchInput]);

  useEffect(() => {
    const element = listViewportRef.current;
    if (!element) {
      return;
    }
    const updateSize = () => {
      setListSize({ height: element.clientHeight, width: element.clientWidth });
    };
    updateSize();
    const observer = new ResizeObserver(updateSize);
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    loadMessages("reset");
  }, [loadMessages]);

  useEffect(() => {
    if (!selectedId || !user) {
      setSelectedMessage(emptyMessage);
      setDetailError(null);
      return;
    }
    setSelectedMessage(emptyMessage);
    setDetailError(null);
    getMessage(selectedId)
      .then((message) => {
        setSelectedMessage(message);
        setDetailTab(defaultTab(message));
        setRawContent("");
      })
      .catch((err) => {
        setDetailError(err instanceof Error ? err.message : "Unable to load message");
        setSelectedMessage(emptyMessage);
      });
  }, [selectedId, user]);

  useEffect(() => {
    if (!selectedId || detailTab !== "raw") {
      return;
    }
    getRawMessage(selectedId)
      .then(setRawContent)
      .catch((err) => setRawContent(err instanceof Error ? err.message : ""));
  }, [detailTab, selectedId]);

  useEffect(() => {
    if (!user) {
      return undefined;
    }
    const source = new EventSource("/api/stream", { withCredentials: true });
    source.addEventListener("message", () => {
      loadMessages("reset");
    });
    return () => source.close();
  }, [loadMessages, user]);

  const handleLogin = async (email: string) => {
    const nextUser = await login(email);
    setUser(nextUser);
    setBox("inbox");
    setMessages([]);
    setSelectedId(null);
    setSearchInput("");
    setCursor(null);
    setHasMore(true);
  };

  const handleSwitch = async (email: string) => {
    await handleLogin(email);
    setSwitchOpen(false);
  };

  const handleLogout = async () => {
    await logout();
    setUser(null);
    setMessages([]);
    setSelectedId(null);
    setCursor(null);
    setHasMore(true);
  };

  const handleDelete = async () => {
    if (!selectedId) {
      return;
    }
    await deleteMessage(selectedId);
    setSelectedId(null);
    loadMessages("reset");
  };

  const handleSend = async (payload: {
    to: string;
    subject: string;
    text: string;
    html: string;
  }) => {
    const recipients = payload.to
      .split(",")
      .map((item) => item.trim())
      .filter(Boolean);
    if (recipients.length === 0) {
      throw new Error("Add at least one recipient.");
    }
    if (!payload.text.trim() && !payload.html.trim()) {
      throw new Error("Add a text or HTML body.");
    }
    await sendMessage({
      to: recipients,
      subject: payload.subject.trim(),
      text: payload.text,
      html: payload.html,
    });
    setComposeOpen(false);
  };

  const formatDate = useMemo(() => {
    return (value: string) => dateFormatter.format(new Date(value));
  }, []);

  const formatLongDate = useMemo(() => {
    return (value: string) => longDateFormatter.format(new Date(value));
  }, []);

  const detailLoading = selectedId !== null && selectedMessage.id !== selectedId && !detailError;

  const rowData = useMemo(
    () => ({
      messages,
      selectedId,
      box,
      hasMore,
      loadingMore,
      formatDate,
      onSelect: (id: string) => setSelectedId(id),
    }),
    [messages, selectedId, box, hasMore, loadingMore, formatDate]
  );

  const handleItemsRendered = useCallback(
    (info: ListOnItemsRenderedProps) => {
      if (!hasMore || loading || loadingMore) {
        return;
      }
      if (info.visibleStopIndex >= messages.length - 2) {
        loadMessages("append");
      }
    },
    [hasMore, loading, loadingMore, loadMessages, messages.length]
  );

  if (!user) {
    return <LoginScreen onLogin={handleLogin} />;
  }

  return (
    <div className="app">
      <div className="shell">
        <header className="topbar">
          <div>
            <p className="brand">LocalSMTP</p>
            <p className="subtitle">Inbox capture for local SMTP testing</p>
          </div>
          <div className="topbar-actions">
            <div className="user-pill">
              <span>Signed in as</span>
              <strong>{user.email}</strong>
            </div>
            <button className="button ghost" onClick={() => setSwitchOpen(true)}>
              Switch user
            </button>
            <button className="button ghost" onClick={handleLogout}>
              Sign out
            </button>
            <button className="button primary" onClick={() => setComposeOpen(true)}>
              Send test email
            </button>
          </div>
        </header>

        <div className="content">
          <aside className="panel">
            <div className="panel-header">
              <div className="tab-group">
                <button
                  className={`tab ${box === "inbox" ? "active" : ""}`}
                  onClick={() => setBox("inbox")}
                >
                  Inbox
                </button>
                <button
                  className={`tab ${box === "sent" ? "active" : ""}`}
                  onClick={() => setBox("sent")}
                >
                  Sent
                </button>
              </div>
              <input
                className="search"
                placeholder="Search subject or address"
                value={searchInput}
                onChange={(event) => setSearchInput(event.target.value)}
              />
            </div>
            <div className="panel-body">
              <div className="list-viewport" ref={listViewportRef}>
                {loading && messages.length === 0 ? (
                  <div className="state">Loading messages...</div>
                ) : null}
                {!loading && messages.length === 0 ? (
                  <div className="state">
                    <p>No messages yet.</p>
                    <p>Send mail to any address and it will appear here.</p>
                  </div>
                ) : null}
                {messages.length > 0 && listSize.height > 0 && listSize.width > 0 ? (
                  <List
                    ref={listRef}
                    height={listSize.height}
                    width={listSize.width}
                    itemCount={hasMore ? messages.length + 1 : messages.length}
                    itemSize={rowHeight}
                    itemData={rowData}
                    onItemsRendered={handleItemsRendered}
                    itemKey={(index, data) => data.messages[index]?.id ?? `loader-${index}`}
                  >
                    {MessageRow}
                  </List>
                ) : null}
              </div>
              {error && <div className="state error">{error}</div>}
            </div>
          </aside>

          <main className="detail">
            {!selectedId ? (
              <div className="detail-empty">
                <h2>Pick a message</h2>
                <p>Choose a message from the list to see its full content.</p>
              </div>
            ) : detailError ? (
              <div className="detail-empty">
                <h2>Unable to load message</h2>
                <p>{detailError}</p>
              </div>
            ) : detailLoading ? (
              <div className="detail-empty">
                <h2>Loading message...</h2>
                <p>Fetching the latest content.</p>
              </div>
            ) : (
              <div className="detail-card">
                <div className="detail-header">
                  <div>
                    <p className="detail-title">
                      {selectedMessage.subject || "(No subject)"}
                    </p>
                    <p className="detail-meta">
                      <span>From</span> {selectedMessage.from}
                    </p>
                    <p className="detail-meta">
                      <span>To</span> {selectedMessage.to.join(", ") || "(none)"}
                    </p>
                    {selectedMessage.cc.length > 0 && (
                      <p className="detail-meta">
                        <span>Cc</span> {selectedMessage.cc.join(", ")}
                      </p>
                    )}
                    {selectedMessage.bcc.length > 0 && (
                      <p className="detail-meta">
                        <span>Bcc</span> {selectedMessage.bcc.join(", ")}
                      </p>
                    )}
                    <p className="detail-meta">
                      <span>Received</span>
                      {selectedMessage.createdAt ? formatLongDate(selectedMessage.createdAt) : ""}
                    </p>
                  </div>
                  <div className="detail-actions">
                    <button className="button ghost" onClick={handleDelete}>
                      Delete
                    </button>
                  </div>
                </div>

                <div className="detail-tabs">
                  <button
                    className={`tab ${detailTab === "html" ? "active" : ""}`}
                    onClick={() => setDetailTab("html")}
                  >
                    HTML
                  </button>
                  <button
                    className={`tab ${detailTab === "text" ? "active" : ""}`}
                    onClick={() => setDetailTab("text")}
                  >
                    Text
                  </button>
                  <button
                    className={`tab ${detailTab === "raw" ? "active" : ""}`}
                    onClick={() => setDetailTab("raw")}
                  >
                    Raw
                  </button>
                </div>

                <div className="detail-body">
                  {detailTab === "html" && selectedMessage.html ? (
                    <div
                      className="message-html"
                      dangerouslySetInnerHTML={{ __html: selectedMessage.html }}
                    />
                  ) : null}
                  {detailTab === "text" && selectedMessage.text ? (
                    <pre className="message-text">{selectedMessage.text}</pre>
                  ) : null}
                  {detailTab === "raw" ? (
                    <pre className="message-text">{rawContent || "Loading raw message..."}</pre>
                  ) : null}
                  {detailTab === "html" && !selectedMessage.html ? (
                    <div className="state">No HTML part in this message.</div>
                  ) : null}
                  {detailTab === "text" && !selectedMessage.text ? (
                    <div className="state">No plain text part in this message.</div>
                  ) : null}
                </div>

                {selectedMessage.attachments.length > 0 && (
                  <div className="attachments">
                    <p className="section-title">Attachments</p>
                    <div className="attachment-list">
                      {selectedMessage.attachments.map((attachment) => (
                        <a
                          key={attachment.id}
                          className="attachment"
                          href={`/api/messages/${selectedMessage.id}/attachments/${attachment.id}`}
                        >
                          <div>
                            <p>{attachment.filename}</p>
                            <span>{attachment.contentType}</span>
                          </div>
                          <span className="attachment-size">
                            {formatBytes(attachment.size)}
                          </span>
                        </a>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            )}
          </main>
        </div>
      </div>

      <ComposeDrawer
        open={composeOpen}
        from={user.email}
        onClose={() => setComposeOpen(false)}
        onSend={handleSend}
      />
      <SwitchUserModal
        open={switchOpen}
        current={user.email}
        onClose={() => setSwitchOpen(false)}
        onSwitch={handleSwitch}
      />
    </div>
  );
}

type RowData = {
  messages: MessageSummary[];
  selectedId: string | null;
  box: Box;
  hasMore: boolean;
  loadingMore: boolean;
  formatDate: (value: string) => string;
  onSelect: (id: string) => void;
};

function MessageRow({
  index,
  style,
  data,
}: ListChildComponentProps<RowData>) {
  if (index >= data.messages.length) {
    return (
      <div style={style} className="list-row">
        <div className="list-footer">
          {data.loadingMore ? "Loading more..." : "Scroll to load more"}
        </div>
      </div>
    );
  }

  const message = data.messages[index];
  const label =
    data.box === "inbox"
      ? message.from
      : message.to.join(", ") || "No recipients";

  return (
    <div style={style} className="list-row">
      <button
        className={`message-item ${data.selectedId === message.id ? "selected" : ""}`}
        onClick={() => data.onSelect(message.id)}
      >
        <div className="message-title">
          <span>{label}</span>
          {message.hasAttachments && <span className="tag">Attachments</span>}
        </div>
        <div className="message-subject">{message.subject || "(No subject)"}</div>
        <div className="message-meta">{data.formatDate(message.createdAt)}</div>
      </button>
    </div>
  );
}

function defaultTab(message: MessageDetail): ViewMode {
  if (message.html) {
    return "html";
  }
  if (message.text) {
    return "text";
  }
  return "raw";
}

function formatBytes(size: number): string {
  if (size < 1024) {
    return `${size} B`;
  }
  if (size < 1024 * 1024) {
    return `${(size / 1024).toFixed(1)} KB`;
  }
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}

function LoginScreen({ onLogin }: { onLogin: (email: string) => Promise<void> }) {
  const [email, setEmail] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setLoading(true);
    setError(null);
    try {
      await onLogin(email);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to login");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="login-screen">
      <div className="login-card">
        <p className="brand">LocalSMTP</p>
        <h1>Capture and explore mail locally</h1>
        <p className="login-text">
          Enter any email to create a mailbox view. No password, no signup.
        </p>
        <form className="login-form" onSubmit={handleSubmit}>
          <input
            type="text"
            inputMode="email"
            placeholder="you@localsmtp.dev"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            required
          />
          <button className="button primary" type="submit" disabled={loading}>
            {loading ? "Signing in..." : "Enter inbox"}
          </button>
        </form>
        {error && <p className="state error">{error}</p>}
      </div>
    </div>
  );
}

function ComposeDrawer({
  open,
  from,
  onClose,
  onSend,
}: {
  open: boolean;
  from: string;
  onClose: () => void;
  onSend: (payload: { to: string; subject: string; text: string; html: string }) => Promise<void>;
}) {
  const [to, setTo] = useState("");
  const [subject, setSubject] = useState("");
  const [text, setText] = useState("");
  const [html, setHtml] = useState("");
  const [sending, setSending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setTo("");
      setSubject("");
      setText("");
      setHtml("");
      setError(null);
      setSending(false);
    }
  }, [open]);

  if (!open) {
    return null;
  }

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setSending(true);
    setError(null);
    try {
      await onSend({ to, subject, text, html });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to send");
    } finally {
      setSending(false);
    }
  };

  return (
    <div className="overlay">
      <div className="drawer">
        <div className="drawer-header">
          <div>
            <p className="drawer-title">Send a local test email</p>
            <p className="drawer-subtitle">Everything stays inside LocalSMTP.</p>
          </div>
          <button className="button ghost" onClick={onClose}>
            Close
          </button>
        </div>
        <form className="drawer-form" onSubmit={handleSubmit}>
          <div className="field">
            <label>From</label>
            <input value={from} readOnly />
          </div>
          <div className="field">
            <label>To</label>
            <input
              value={to}
              onChange={(event) => setTo(event.target.value)}
              placeholder="one@localsmtp.dev, two@localsmtp.dev"
              required
            />
          </div>
          <div className="field">
            <label>Subject</label>
            <input
              value={subject}
              onChange={(event) => setSubject(event.target.value)}
              placeholder="Subject"
            />
          </div>
          <div className="field">
            <label>Plain text</label>
            <textarea
              rows={5}
              value={text}
              onChange={(event) => setText(event.target.value)}
              placeholder="Hello from LocalSMTP"
            />
          </div>
          <div className="field">
            <label>HTML (optional)</label>
            <textarea
              rows={5}
              value={html}
              onChange={(event) => setHtml(event.target.value)}
              placeholder="<strong>Optional HTML content</strong>"
            />
          </div>
          {error && <p className="state error">{error}</p>}
          <div className="drawer-actions">
            <button className="button ghost" type="button" onClick={onClose}>
              Cancel
            </button>
            <button className="button primary" type="submit" disabled={sending}>
              {sending ? "Sending..." : "Send email"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

function SwitchUserModal({
  open,
  current,
  onClose,
  onSwitch,
}: {
  open: boolean;
  current: string;
  onClose: () => void;
  onSwitch: (email: string) => Promise<void>;
}) {
  const [email, setEmail] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setEmail("");
      setError(null);
      setLoading(false);
    }
  }, [open]);

  if (!open) {
    return null;
  }

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setLoading(true);
    setError(null);
    try {
		await onSwitch(email.trim());
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to switch");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="overlay">
      <div className="modal">
        <div className="modal-header">
          <p className="drawer-title">Switch mailbox</p>
          <p className="drawer-subtitle">Use any email to view its inbox.</p>
        </div>
        <form className="drawer-form" onSubmit={handleSubmit}>
          <div className="field">
            <label>New email</label>
            <input
              type="text"
              inputMode="email"
              placeholder={current}
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              required
            />
          </div>
          {error && <p className="state error">{error}</p>}
          <div className="drawer-actions">
            <button className="button ghost" type="button" onClick={onClose}>
              Cancel
            </button>
            <button className="button primary" type="submit" disabled={loading}>
              {loading ? "Switching..." : "Switch user"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
