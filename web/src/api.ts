import type { MessageDetail, MessageSummary, User } from "./types";

type MessageListResponse = {
  messages: MessageSummary[];
  page: number;
  limit: number;
  total: number;
  hasMore: boolean;
  nextPage: number;
};

const headers = {
  "Content-Type": "application/json",
};

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    credentials: "include",
    ...options,
    headers: {
      ...headers,
      ...(options.headers || {}),
    },
  });

  if (!response.ok) {
    const message = await response.text();
    throw new Error(message || response.statusText);
  }
  if (response.status === 204) {
    return undefined as T;
  }
  return response.json() as Promise<T>;
}

export async function getMe(): Promise<User> {
  return request<User>("/api/me");
}

export async function login(email: string): Promise<User> {
  return request<User>("/api/login", {
    method: "POST",
    body: JSON.stringify({ email }),
  });
}

export async function logout(): Promise<void> {
  await request<void>("/api/logout", { method: "POST" });
}

export async function listMessages(
  box: string,
  search: string,
  page: number,
  limit: number
): Promise<MessageListResponse> {
  const params = new URLSearchParams();
  params.set("box", box);
  if (search) {
    params.set("search", search);
  }
  params.set("page", String(page));
  params.set("limit", String(limit));
  return request<MessageListResponse>(`/api/messages?${params.toString()}`);
}

export async function getMessage(id: string): Promise<MessageDetail> {
  return request<MessageDetail>(`/api/messages/${id}`);
}

export async function deleteMessage(id: string): Promise<void> {
  await request<void>(`/api/messages/${id}`, { method: "DELETE" });
}

export async function getRawMessage(id: string): Promise<string> {
  const response = await fetch(`/api/messages/${id}/raw`, { credentials: "include" });
  if (!response.ok) {
    const message = await response.text();
    throw new Error(message || response.statusText);
  }
  return response.text();
}

export async function sendMessage(payload: {
  to: string[];
  subject: string;
  text: string;
  html: string;
}): Promise<void> {
  await request<void>("/api/send", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}
