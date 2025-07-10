import { useCallback } from 'react';
import { errorReportingService, ErrorContext } from '../services/errorReporting';

export interface UseErrorHandlerOptions {
  component?: string;
  context?: string;
}

export const useErrorHandler = (options: UseErrorHandlerOptions = {}) => {
  const reportError = useCallback((error: Error, additionalContext?: ErrorContext) => {
    const context: ErrorContext = {
      ...additionalContext,
      context: additionalContext?.context || options.context,
      metadata: {
        ...additionalContext?.metadata,
        component: options.component,
      },
    };

    errorReportingService.notify(error, context);
  }, [options.component, options.context]);

  const reportErrorMessage = useCallback((message: string, additionalContext?: ErrorContext) => {
    const context: ErrorContext = {
      ...additionalContext,
      context: additionalContext?.context || options.context,
      metadata: {
        ...additionalContext?.metadata,
        component: options.component,
      },
    };

    errorReportingService.notifyError(message, context);
  }, [options.component, options.context]);

  const handleAsyncError = useCallback(async <T>(
    asyncFn: () => Promise<T>,
    errorContext?: ErrorContext
  ): Promise<T | null> => {
    try {
      return await asyncFn();
    } catch (error) {
      if (error instanceof Error) {
        reportError(error, errorContext);
      } else {
        reportErrorMessage(`Unknown error: ${String(error)}`, errorContext);
      }
      return null;
    }
  }, [reportError, reportErrorMessage]);

  const leaveBreadcrumb = useCallback((message: string, metadata?: Record<string, unknown>) => {
    const enrichedMetadata = {
      ...metadata,
      component: options.component,
    };
    
    errorReportingService.leaveBreadcrumb(message, enrichedMetadata);
  }, [options.component]);

  const setUserContext = useCallback((userId: string, email?: string, name?: string) => {
    errorReportingService.setUser(userId, email, name);
  }, []);

  const addMetadata = useCallback((section: string, key: string, value: unknown) => {
    errorReportingService.addMetadata(section, key, value);
  }, []);

  const clearMetadata = useCallback((section: string, key?: string) => {
    errorReportingService.clearMetadata(section, key);
  }, []);

  return {
    reportError,
    reportErrorMessage,
    handleAsyncError,
    leaveBreadcrumb,
    setUserContext,
    addMetadata,
    clearMetadata,
  };
};

export default useErrorHandler;