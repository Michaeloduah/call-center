import { Alert } from "@/components/ui/Alert";
import { actionErrorText } from "@/lib/actionErrors";

export function ActionError({ value }: { value: string | string[] | undefined }) {
  const text = actionErrorText(value);
  return text ? (
    <div className="mb-6">
      <Alert tone="error">{text}</Alert>
    </div>
  ) : null;
}
