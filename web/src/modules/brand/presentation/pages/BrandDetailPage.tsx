import { useParams } from "react-router";
import { useCan } from "@/shared/lib/session";
import { Badge } from "@/shared/ui/badge";
import { ErrorState, PageHeader } from "@/shared/ui/patterns";
import { Skeleton } from "@/shared/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/shared/ui/tabs";
import { useBrand } from "../../application/useBrands";
import { AssetsTab } from "../components/AssetsTab";
import { ProductsTab } from "../components/ProductsTab";
import { ProfileForm } from "../components/ProfileForm";
import { VersionsTab } from "../components/VersionsTab";

export default function BrandDetailPage() {
  const { brandId = "" } = useParams();
  const q = useBrand(brandId);
  const canManage = useCan("brand.manage");

  if (q.isLoading) {
    return (
      <div className="flex flex-col gap-4" aria-busy>
        <Skeleton className="h-7 w-64" />
        <Skeleton className="h-8 w-96" />
        <Skeleton className="h-64 w-full max-w-3xl" />
      </div>
    );
  }
  if (q.error || !q.data) return <ErrorState error={q.error} onRetry={() => void q.refetch()} />;
  const brand = q.data;

  return (
    <div>
      <PageHeader title={brand.name} meta={<Badge>Phiên bản {brand.currentVersion.versionNo}</Badge>} description="Mỗi lần lưu hồ sơ tạo một phiên bản mới; job AI ghi lại phiên bản đã dùng." />
      <Tabs defaultValue="profile">
        <TabsList>
          <TabsTrigger value="profile">Hồ sơ</TabsTrigger>
          <TabsTrigger value="products">Sản phẩm</TabsTrigger>
          <TabsTrigger value="assets">Tài nguyên</TabsTrigger>
          <TabsTrigger value="versions">Lịch sử phiên bản</TabsTrigger>
        </TabsList>
        <TabsContent value="profile" className="pt-4">
          <ProfileForm brand={brand} readOnly={!canManage} onReload={() => void q.refetch()} />
        </TabsContent>
        <TabsContent value="products" className="pt-4">
          <ProductsTab brandId={brand.id} canManage={canManage} />
        </TabsContent>
        <TabsContent value="assets" className="pt-4">
          <AssetsTab brandId={brand.id} canManage={canManage} />
        </TabsContent>
        <TabsContent value="versions" className="pt-4">
          <VersionsTab brandId={brand.id} currentVersionNo={brand.currentVersion.versionNo} />
        </TabsContent>
      </Tabs>
    </div>
  );
}
