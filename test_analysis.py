from edge_and_anomaly_detection import EdgeDetector

# Create detector
detector = EdgeDetector()

# Run complete analysis
result = detector.analyze_complete(
    video_path='bumrah_bowling.mp4',
    pose_csv_path='reference_data/cricket/Bumrah.csv',
    sport_type='cricket'
)

print("\n✅ ANALYSIS COMPLETE!")
print("Result Dictionary for FastAPI:", result)