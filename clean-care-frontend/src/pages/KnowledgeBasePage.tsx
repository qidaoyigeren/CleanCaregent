import { useState, type ChangeEvent } from 'react';
import { Link } from 'react-router-dom';
import { uploadDocument } from '../api/knowledge';
import KnowledgeSearch from '../components/admin/KnowledgeSearch';
import ErrorMessage from '../components/ui/ErrorMessage';

export default function KnowledgeBasePage() {
  const [uploadMsg, setUploadMsg] = useState<string | null>(null);
  const [uploadError, setUploadError] = useState<string | null>(null);
  const [docType, setDocType] = useState('user_manual');

  const handleFileUpload = async (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    const formData = new FormData();
    formData.append('file', file);
    // Backend expects these fields for document metadata
    formData.append('title', file.name.replace(/\.[^.]+$/, ''));
    formData.append('doc_type', docType);
    formData.append('category', 'general');

    setUploadMsg(null);
    setUploadError(null);

    try {
      const result = await uploadDocument(formData);
      setUploadMsg(`Document uploaded: ${result.document_id}`);
      e.target.value = '';
    } catch (err) {
      setUploadError((err as Error).message || 'Upload failed');
    }
  };

  return (
    <div className="admin-page">
      <nav className="admin-nav">
        <Link to="/admin" className="admin-nav__link">Overview</Link>
        <Link to="/admin/kb" className="admin-nav__link admin-nav__link--active">Knowledge Base</Link>
        <Link to="/admin/eval" className="admin-nav__link">Evaluation</Link>
        <Link to="/admin/prompts" className="admin-nav__link">Prompts</Link>
      </nav>

      <h1 className="admin-page__title">Knowledge Base</h1>

      {/* Upload Area */}
      <h3 className="section-header">Upload Document</h3>
      <label className="eval-form__field" style={{ marginBottom: 'var(--space-3)' }}>
        <span className="eval-form__label">Document Type</span>
        <select className="eval-form__input" value={docType} onChange={(event) => setDocType(event.target.value)}>
          {[
            'product_detail', 'product_parameter', 'product_comparison',
            'purchase_guide', 'accessory_compatibility', 'user_manual',
            'troubleshooting', 'after_sales_policy', 'faq',
          ].map((value) => <option key={value}>{value}</option>)}
        </select>
      </label>
      <label htmlFor="kb-file-upload" className="upload-area" style={{ display: 'block', cursor: 'pointer' }}>
        <input
          id="kb-file-upload"
          type="file"
          accept=".json,.csv,.xlsx,.md,.txt,.html,.docx,.pdf"
          onChange={handleFileUpload}
          style={{ display: 'none' }}
        />
        <div className="upload-area__icon">{'📁'}</div>
        <div className="upload-area__text">
          Drop files here or click to upload (.json, .csv, .xlsx, .md, .txt, .html, .docx, .pdf)
        </div>
      </label>

      {uploadMsg && (
        <div className="error-message" style={{ background: 'var(--color-success-light)', borderColor: '#bbf7d0', color: 'var(--color-success)', marginBottom: 'var(--space-4)' }}>
          <span>{'✓'}</span>
          <span className="error-message__text">{uploadMsg}</span>
        </div>
      )}
      {uploadError && (
        <div style={{ marginBottom: 'var(--space-4)' }}>
          <ErrorMessage message={uploadError} onRetry={() => setUploadError(null)} />
        </div>
      )}

      {/* Search */}
      <h3 className="section-header" style={{ marginTop: 'var(--space-6)' }}>Search Knowledge</h3>
      <KnowledgeSearch />
    </div>
  );
}
