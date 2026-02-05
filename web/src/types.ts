export type User = {
  email: string;
  emails: string[];
};

export type AccountSummary = {
  email: string;
  unread: number;
};

export type MessageSummary = {
  id: string;
  from: string;
  to: string[];
  subject: string;
  createdAt: string;
  hasAttachments: boolean;
};

export type Attachment = {
  id: number;
  filename: string;
  contentType: string;
  size: number;
};

export type MessageDetail = {
  id: string;
  from: string;
  to: string[];
  cc: string[];
  bcc: string[];
  subject: string;
  text: string;
  html: string;
  createdAt: string;
  rawSize: number;
  attachments: Attachment[];
};
