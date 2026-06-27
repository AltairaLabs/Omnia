interface FieldErrorProps {
  message?: string | null;
  id?: string;
}

/** Inline field-level validation message. Renders nothing when valid. */
export function FieldError({ message, id }: FieldErrorProps) {
  if (!message) return null;
  return (
    <p id={id} role="alert" className="mt-1 text-sm text-destructive">
      {message}
    </p>
  );
}
