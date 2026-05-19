import type { AuthResponse, User } from "../../types/api";
import { apiRequest } from "./client";

export const authService = {
  login(email: string, password: string) {
    return apiRequest<AuthResponse>("/auth/login", {
      auth: false,
      method: "POST",
      body: { email, password }
    });
  },

  register(email: string, password: string) {
    return apiRequest<AuthResponse>("/auth/register", {
      auth: false,
      method: "POST",
      body: { email, password }
    });
  },

  me() {
    return apiRequest<{ user: User }>("/auth/me");
  }
};
