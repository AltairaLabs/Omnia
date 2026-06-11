"use client";

import { useState } from "react";
import { AlertTriangle } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { useToast } from "@/hooks/core";
import { useChangeEmbeddingDimension } from "@/hooks/use-embedding-dimension";

const MIN_DIM = 1;
const MAX_DIM = 2000;

interface AdminTabProps {
  workspaceName: string;
}

export function AdminTab({ workspaceName }: AdminTabProps) {
  const [dimension, setDimension] = useState("");
  const { toast } = useToast();
  const change = useChangeEmbeddingDimension(workspaceName);

  const parsed = Number.parseInt(dimension, 10);
  const valid = Number.isInteger(parsed) && parsed >= MIN_DIM && parsed <= MAX_DIM;

  function handleConfirm() {
    change.mutate(parsed, {
      onSuccess: () => {
        toast({
          title: "Consent recorded",
          description: `The memory embedding dimension will change to ${parsed} on the next memory-api restart. Existing embeddings are discarded and re-embedded.`,
        });
        setDimension("");
      },
      onError: (err: Error) => {
        toast({
          title: "Failed to record consent",
          description: err.message,
          variant: "destructive",
        });
      },
    });
  }

  return (
    <div className="space-y-6">
      <Card className="border-destructive/50">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <AlertTriangle className="size-4 text-destructive" />
            Change Memory Embedding Dimension
          </CardTitle>
          <CardDescription>
            Records one-shot consent to resize the memory embedding vectors so a
            different embedding model can back this workspace&apos;s memory. This is
            destructive: every stored embedding is discarded and re-embedded on the next
            memory-api restart, and semantic recall is degraded until the backfill
            completes. Consent is applied once, then cleared.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2 max-w-xs">
            <Label htmlFor="embedding-dimension">New dimension</Label>
            <Input
              id="embedding-dimension"
              type="number"
              min={MIN_DIM}
              max={MAX_DIM}
              placeholder="e.g. 768"
              value={dimension}
              onChange={(e) => setDimension(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              Must match the configured embedding provider&apos;s output (1–{MAX_DIM}). e.g.
              nomic-embed-text = 768, mxbai-embed-large = 1024, OpenAI = 1536.
            </p>
          </div>
          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button variant="destructive" disabled={!valid || change.isPending}>
                Record dimension change
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>Discard all embeddings?</AlertDialogTitle>
                <AlertDialogDescription>
                  This authorises changing the embedding dimension to{" "}
                  <span className="font-mono">{valid ? parsed : "?"}</span>. On the next
                  memory-api restart every stored embedding in this workspace is dropped and
                  re-embedded from scratch. Semantic recall is degraded until the backfill
                  finishes. This cannot be undone.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Cancel</AlertDialogCancel>
                <AlertDialogAction
                  onClick={handleConfirm}
                  className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                >
                  Record consent
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </CardContent>
      </Card>
    </div>
  );
}
