// Stub auth types - no auth in loom

export interface User {
  id: string;
  email: string;
  name: string;
  avatarUrl?: string;
  githubUsername?: string;
  role?: string;
  expertise?: string;
  provider: 'google' | 'github' | 'dev';
  preferences?: {
    [key: string]: unknown;
  };
}

export interface UserProfileUpdate {
  name?: string;
  githubUsername?: string;
  role?: string;
  expertise?: string;
}

export interface AuthTokens {
  accessToken: string;
  refreshToken: string;
}

export interface AuthError {
  message: string;
  code?: string;
}
