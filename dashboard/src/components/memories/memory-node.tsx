/**
 * MemoryNode — custom @xyflow node rendered as a colored circle/bubble.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { memo } from "react";
import { Handle, Position, type NodeProps } from "@xyflow/react";
import { getCategoryColor } from "./category-badge";

export interface MemoryNodeData {
  label: string;
  category?: string;
  confidence: number;
  memoryId: string;
  [key: string]: unknown; // required by @xyflow
}

function MemoryNodeComponent({ data }: NodeProps) {
  const nodeData = data as MemoryNodeData;
  const size = 30 + nodeData.confidence * 50; // 30px to 80px based on confidence
  const color = getCategoryColor(nodeData.category);
  const displayLabel =
    nodeData.label.length > 15 ? nodeData.label.slice(0, 12) + "..." : nodeData.label;

  return (
    <div
      data-testid="memory-node"
      className="flex items-center justify-center rounded-full cursor-pointer transition-shadow hover:shadow-lg hover:shadow-primary/20"
      style={{
        width: size,
        height: size,
        backgroundColor: color,
        opacity: 0.85,
      }}
      title={nodeData.label}
    >
      <span className="text-white text-[10px] font-medium max-w-[80%] truncate text-center leading-tight px-1">
        {displayLabel}
      </span>
      <Handle type="source" position={Position.Right} className="!opacity-0 !w-0 !h-0" />
      <Handle type="target" position={Position.Left} className="!opacity-0 !w-0 !h-0" />
    </div>
  );
}

export const MemoryNode = memo(MemoryNodeComponent);
