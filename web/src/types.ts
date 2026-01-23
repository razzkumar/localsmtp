export type User = {
  email: string;
};

export type MessageSummary = {
  id: string;
  from: string;
  to: string[];
  subject: string;
  preview: string;
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
