import { FileDirectoryIcon, FileIcon, FileMediaIcon, FileZipIcon, PackageIcon } from "@primer/octicons-react";

const icons = {
  video: FileMediaIcon,
  audio: FileMediaIcon,
  archive: FileZipIcon,
  document: FileDirectoryIcon,
  app: PackageIcon,
  other: FileIcon,
} as const;

/** CategoryIcon shows what kind of file a download holds. */
export function CategoryIcon({ category }: { category: string }) {
  const Icon = icons[category as keyof typeof icons] ?? FileIcon;
  return (
    <span className={`cat-icon cat-${category}`}>
      <Icon size={16} aria-hidden />
    </span>
  );
}
