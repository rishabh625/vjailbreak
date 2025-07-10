import type { Client } from '@bugsnag/js';

export interface ErrorMetadata {
  userId?: string;
  userEmail?: string;
  component?: string;
  action?: string;
  migrationId?: string;
  nodeId?: string;
  [key: string]: unknown;
}

export interface ErrorContext {
  severity?: 'error' | 'warning' | 'info';
  context?: string;
  metadata?: ErrorMetadata;
}

class ErrorReportingService {
  private client: Client | null = null;
  private isInitialized = false;

  initialize(client: Client): void {
    this.client = client;
    this.isInitialized = true;
  }

  isReady(): boolean {
    return this.isInitialized && this.client !== null;
  }

  setUser(userId: string, email?: string, name?: string): void {
    if (!this.isReady()) return;
    
    this.client!.setUser(userId, email, name);
  }

  addMetadata(section: string, key: string, value: unknown): void {
    if (!this.isReady()) return;
    
    this.client!.addMetadata(section, key, value);
  }

  clearMetadata(section: string, key?: string): void {
    if (!this.isReady()) return;
    
    this.client!.clearMetadata(section, key);
  }

  notify(error: Error, context?: ErrorContext): void {
    if (!this.isReady()) {
      console.error('ErrorReportingService not initialized:', error);
      return;
    }

    this.client!.notify(error, (event) => {
      if (context?.severity) {
        event.severity = context.severity;
      }
      
      if (context?.context) {
        event.context = context.context;
      }
      
      if (context?.metadata) {
        Object.entries(context.metadata).forEach(([key, value]) => {
          event.addMetadata('custom', key, value);
        });
      }
    });
  }

  notifyError(message: string, context?: ErrorContext): void {
    this.notify(new Error(message), context);
  }

  leaveBreadcrumb(message: string, metadata?: Record<string, unknown>): void {
    if (!this.isReady()) return;
    
    this.client!.leaveBreadcrumb(message, metadata);
  }

  addFeatureFlag(name: string, variant?: string): void {
    if (!this.isReady()) return;
    
    this.client!.addFeatureFlag(name, variant);
  }

  clearFeatureFlag(name: string): void {
    if (!this.isReady()) return;
    
    this.client!.clearFeatureFlag(name);
  }

  startSession(): void {
    if (!this.isReady()) return;
    
    this.client!.startSession();
  }

  pauseSession(): void {
    if (!this.isReady()) return;
    
    this.client!.pauseSession();
  }

  resumeSession(): void {
    if (!this.isReady()) return;
    
    this.client!.resumeSession();
  }
}

export const errorReportingService = new ErrorReportingService();
export default errorReportingService;