import React, { useState } from "react";
import { Dialog, DialogTitle, DialogContent, DialogActions, Button, TextField, CircularProgress } from "@mui/material";

interface ExcelMigrationModalProps {
  open: boolean;
  onClose: () => void;
}

export default function ExcelMigrationModal({ open, onClose }: ExcelMigrationModalProps) {
  const [file, setFile] = useState<File | null>(null);
  const [uploading, setUploading] = useState(false);
  const [responseMessage, setResponseMessage] = useState<string | null>(null);

  const handleFileChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    if (event.target.files && event.target.files[0]) {
      setFile(event.target.files[0]);
    }
  };

  const handleSubmit = async () => {
    if (!file) return;

    setUploading(true);
    const formData = new FormData();
    formData.append("excelFile", file);

    try {
      const response = await fetch("http://localhost:8080/upload", {
        method: "POST",
        body: formData,
      });
      const result = await response.text();
      setResponseMessage(result);
    } catch (error) {
      alert(error)
      setResponseMessage("Failed to upload file.");
    } finally {
      setUploading(false);
    }
  };

  const handleClose = () => {
    setResponseMessage(null); // Clear the response message
    onClose(); // Close the modal
  };

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>Upload Excel File</DialogTitle>
      <DialogContent>
        {responseMessage ? (
          <div>
            <p>{responseMessage}</p>
            <Button onClick={handleClose} color="primary" variant="contained">
              Close
            </Button>
          </div>
        ) : (
          <TextField
            type="file"
            onChange={handleFileChange}
            fullWidth
            InputLabelProps={{ shrink: true }}
          />
        )}
      </DialogContent>
      {!responseMessage && (
        <DialogActions>
          <Button onClick={handleClose} disabled={uploading}>
            Cancel
          </Button>
          <Button onClick={handleSubmit} color="primary" variant="contained" disabled={!file || uploading}>
            {uploading ? <CircularProgress size={20} /> : "Submit"}
          </Button>
        </DialogActions>
      )}
    </Dialog>
  );
}
