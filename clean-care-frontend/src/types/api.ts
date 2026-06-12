/** Generic API response envelope from Go backend */
export interface Envelope<T> {
  code: string;
  message?: string;
  request_id?: string;
  data: T;
}

/** Structured API error */
export class ApiError extends Error {
  code: string;
  statusCode: number;
  requestId?: string;

  constructor(statusCode: number, code: string, message?: string, requestId?: string) {
    super(message || code);
    this.name = 'ApiError';
    this.code = code;
    this.statusCode = statusCode;
    this.requestId = requestId;
  }
}

/** Paginated response */
export interface PaginatedItems<T> {
  items: T[];
  next_cursor?: string;
}
